package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	llamav1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/compare"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy/plugins"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	yamlpkg "sigs.k8s.io/yaml"
)

const deploymentKind = "Deployment"

// RenderManifest takes a manifest directory and transforms it through
// kustomization and plugins to produce final Kubernetes resources.
func RenderManifest(
	fs filesys.FileSystem,
	manifestPath string,
	ownerInstance *llamav1alpha1.LlamaStackDistribution,
) (*resmap.ResMap, error) {
	// fallback to the 'default' directory' if we cannot initially find
	// the kustomization file
	finalManifestPath := manifestPath
	if exists := fs.Exists(filepath.Join(manifestPath, "kustomization.yaml")); !exists {
		finalManifestPath = filepath.Join(manifestPath, "default")
	}

	k := krusty.MakeKustomizer(krusty.MakeDefaultOptions())

	resMapVal, err := k.Run(fs, finalManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to run kustomize: %w", err)
	}
	if err := applyPlugins(&resMapVal, ownerInstance); err != nil {
		return nil, err
	}
	return &resMapVal, nil
}

// ApplyResources takes a Kustomize ResMap and applies the resources to the cluster.
func ApplyResources(
	ctx context.Context,
	cli client.Client,
	scheme *runtime.Scheme,
	ownerInstance *llamav1alpha1.LlamaStackDistribution,
	resMap *resmap.ResMap,
) error {
	for _, res := range (*resMap).Resources() {
		if err := manageResource(ctx, cli, scheme, res, ownerInstance); err != nil {
			return fmt.Errorf("failed to manage resource %s/%s: %w", res.GetKind(), res.GetName(), err)
		}
	}
	return nil
}

// manageResource acts as a dispatcher, checking if a resource exists and then
// deciding whether to create it or patch it.
func manageResource(
	ctx context.Context,
	cli client.Client,
	scheme *runtime.Scheme,
	res *resource.Resource,
	ownerInstance *llamav1alpha1.LlamaStackDistribution,
) error {
	// prevent the controller from trying to apply changes to its own CR
	if res.GetKind() == llamav1alpha1.LlamaStackDistributionKind && res.GetName() == ownerInstance.Name && res.GetNamespace() == ownerInstance.Namespace {
		return nil
	}

	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(res.MustYaml()), u); err != nil {
		return fmt.Errorf("failed to unmarshal resource: %w", err)
	}

	// Check if RoleBinding references a SCC ClusterRole that exists
	if u.GetKind() == "RoleBinding" {
		if shouldSkip, err := CheckClusterRoleExists(ctx, cli, u); err != nil {
			return fmt.Errorf("failed to check ClusterRole existence: %w", err)
		} else if shouldSkip {
			log.FromContext(ctx).V(1).Info("Skipping RoleBinding - referenced SCC ClusterRole not found",
				"roleBinding", u.GetName())
			return nil
		}
	}

	kGvk := res.GetGvk()
	gvk := schema.GroupVersionKind{
		Group:   kGvk.Group,
		Version: kGvk.Version,
		Kind:    kGvk.Kind,
	}

	found := u.DeepCopy()
	err := cli.Get(ctx, client.ObjectKeyFromObject(u), found)
	if err != nil {
		if !k8serr.IsNotFound(err) {
			return fmt.Errorf("failed to get resource: %w", err)
		}
		return createResource(ctx, cli, u, ownerInstance, scheme, gvk)
	}
	return patchResource(ctx, cli, u, found, ownerInstance)
}

// createResource creates a new resource, setting an owner reference only if it's namespace-scoped.
func createResource(
	ctx context.Context,
	cli client.Client,
	obj *unstructured.Unstructured,
	ownerInstance *llamav1alpha1.LlamaStackDistribution,
	scheme *runtime.Scheme,
	gvk schema.GroupVersionKind,
) error {
	// Check if the resource is cluster-scoped (like a ClusterRole) to avoid
	// incorrectly setting a namespace-bound owner reference on it.
	isClusterScoped, err := isClusterScoped(cli.RESTMapper(), gvk)
	if err != nil {
		return fmt.Errorf("failed to determine resource scope: %w", err)
	}
	if !isClusterScoped {
		if err := ctrl.SetControllerReference(ownerInstance, obj, scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for %s: %w", gvk.Kind, err)
		}
	}
	return cli.Create(ctx, obj)
}

// isClusterScoped checks if a given GVK refers to a cluster-scoped resource.
func isClusterScoped(mapper meta.RESTMapper, gvk schema.GroupVersionKind) (bool, error) {
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, fmt.Errorf("failed to get REST mapping for GVK %v: %w", gvk, err)
	}
	return mapping.Scope.Name() == meta.RESTScopeNameRoot, nil
}

// patchResource patches an existing resource, but only if we own it.
func patchResource(ctx context.Context, cli client.Client, desired, existing *unstructured.Unstructured, ownerInstance *llamav1alpha1.LlamaStackDistribution) error {
	logger := log.FromContext(ctx)

	// Critical safety check to prevent the operator from "stealing" or
	// overwriting a resource that was created by another user or controller.
	isOwner := false
	for _, ref := range existing.GetOwnerReferences() {
		if ref.UID == ownerInstance.GetUID() {
			isOwner = true
			break
		}
	}
	if !isOwner {
		logger.V(1).Info("Skipping resource not owned by this instance",
			"kind", existing.GetKind(),
			"name", existing.GetName(),
			"namespace", existing.GetNamespace())
		return nil
	}

	switch existing.GetKind() {
	case "PersistentVolumeClaim":
		logger.V(1).Info("Skipping PVC patch - PVCs are immutable after creation",
			"name", existing.GetName(),
			"namespace", existing.GetNamespace())
		return nil
	case "Service":
		if err := compare.CheckAndLogServiceChanges(ctx, cli, desired); err != nil {
			return fmt.Errorf("failed to validate resource mutations while patching: %w", err)
		}
	case deploymentKind:
		// Some volume changes cannot be handled by SSA because the volumes were originally
		// created via cli.Create (no SSA field manager tracking), so SSA cannot remove
		// unowned fields. Fall back to full replacement in these cases.
		if reason := deploymentNeedsFullReplacement(ctx, desired, existing); reason != "" {
			logger.Info("Using full replacement instead of SSA for Deployment",
				"deployment", existing.GetName(),
				"namespace", existing.GetNamespace(),
				"reason", reason)
			desired.SetResourceVersion(existing.GetResourceVersion())
			return cli.Update(ctx, desired)
		}
	}

	data, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal desired state: %w", err)
	}

	return cli.Patch(
		ctx,
		existing,
		client.RawPatch(k8stypes.ApplyPatchType, data),
		client.ForceOwnership,
		client.FieldOwner(ownerInstance.GetName()),
	)
}

// applyPlugins runs all Go-based transformations on the resource map.
func applyPlugins(resMap *resmap.ResMap, ownerInstance *llamav1alpha1.LlamaStackDistribution) error {
	namePrefixPlugin := plugins.CreateNamePrefixPlugin(plugins.NamePrefixConfig{
		Prefix: ownerInstance.GetName(),
		// Exclude Deployment to maintain backward compatibility with existing deployment names
		ExcludeKinds: []string{deploymentKind},
	})
	if err := namePrefixPlugin.Transform(*resMap); err != nil {
		return fmt.Errorf("failed to apply name prefix: %w", err)
	}

	namespaceSetterPlugin, err := plugins.CreateNamespacePlugin(ownerInstance.GetNamespace())
	if err != nil {
		return err
	}
	if err := namespaceSetterPlugin.Transform(*resMap); err != nil {
		return fmt.Errorf("failed to apply namespace setter plugin: %w", err)
	}

	fieldTransformerPlugin := plugins.CreateFieldMutator(plugins.FieldMutatorConfig{
		Mappings: getFieldMappings(ownerInstance),
	})
	if err := fieldTransformerPlugin.Transform(*resMap); err != nil {
		return fmt.Errorf("failed to apply field transformer: %w", err)
	}

	// Apply NetworkPolicy transformer to configure ingress rules based on spec.network
	if err := applyNetworkPolicyTransformer(resMap, ownerInstance); err != nil {
		return fmt.Errorf("failed to apply NetworkPolicy transformer: %w", err)
	}

	if isAutoscalingEnabled(ownerInstance) {
		if err := removeDeploymentReplicas(*resMap); err != nil {
			return fmt.Errorf("failed to strip replicas for autoscaling: %w", err)
		}
	}

	return nil
}

// applyNetworkPolicyTransformer applies the NetworkPolicy transformer plugin.
func applyNetworkPolicyTransformer(resMap *resmap.ResMap, ownerInstance *llamav1alpha1.LlamaStackDistribution) error {
	operatorNS, err := GetOperatorNamespace()
	if err != nil {
		operatorNS = "llama-stack-k8s-operator-system"
	}

	npTransformer := plugins.CreateNetworkPolicyTransformer(plugins.NetworkPolicyTransformerConfig{
		InstanceName:      ownerInstance.GetName(),
		ServicePort:       GetServicePort(ownerInstance),
		OperatorNamespace: operatorNS,
		NetworkSpec:       ownerInstance.Spec.Network,
	})

	return npTransformer.Transform(*resMap)
}

// removeDeploymentReplicas deletes spec.replicas from Deployment manifests so that
// the HPA (or default Kubernetes behavior) controls the replica count.
func removeDeploymentReplicas(resMap resmap.ResMap) error {
	for _, res := range resMap.Resources() {
		if res.GetKind() != deploymentKind {
			continue
		}

		data, err := parseResourceYAML(res)
		if err != nil {
			return err
		}

		spec, ok := data["spec"].(map[string]any)
		if !ok {
			continue
		}

		if _, exists := spec["replicas"]; !exists {
			continue
		}

		delete(spec, "replicas")

		if err := updateResourceFromData(res, data); err != nil {
			return err
		}
	}

	return nil
}

// getFieldMappings returns essential field mappings for kustomize transformation.
func getFieldMappings(ownerInstance *llamav1alpha1.LlamaStackDistribution) []plugins.FieldMapping {
	instanceName := ownerInstance.GetName()
	instanceNamespace := ownerInstance.GetNamespace()
	serviceAccountName := instanceName + "-sa"
	servicePort := getServicePort(ownerInstance)
	storageSize := getStorageSize(ownerInstance)
	instanceLabelPath := "/app.kubernetes.io~1instance"

	mappings := buildFieldMappings(instanceName, instanceNamespace, serviceAccountName, servicePort, storageSize, instanceLabelPath, ownerInstance.Spec.Replicas)

	// When persistent storage is configured, use Recreate strategy to avoid
	// RWO PVC multi-attach deadlock during rolling updates
	if ownerInstance.Spec.Server.Storage != nil {
		mappings = append(mappings, plugins.FieldMapping{
			SourceValue:       "Recreate",
			TargetField:       "/spec/strategy/type",
			TargetKind:        "Deployment",
			CreateIfNotExists: true,
		})
	}

	return mappings
}

// buildFieldMappings constructs the field mappings array.
func buildFieldMappings(instanceName, instanceNamespace, serviceAccountName string,
	servicePort any, storageSize, instanceLabelPath string, replicas int32) []plugins.FieldMapping {
	var replicaSourceValue any = replicas
	return []plugins.FieldMapping{
		{
			SourceValue:       storageSize,
			DefaultValue:      llamav1alpha1.DefaultStorageSize.String(),
			TargetField:       "/spec/resources/requests/storage",
			TargetKind:        "PersistentVolumeClaim",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       servicePort,
			DefaultValue:      llamav1alpha1.DefaultServerPort,
			TargetField:       "/spec/ports/0/port",
			TargetKind:        "Service",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       servicePort,
			DefaultValue:      llamav1alpha1.DefaultServerPort,
			TargetField:       "/spec/ports/0/targetPort",
			TargetKind:        "Service",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       instanceName,
			TargetField:       "/spec/selector" + instanceLabelPath,
			TargetKind:        "Service",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       instanceName,
			TargetField:       "/metadata/name",
			TargetKind:        "Deployment",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       replicaSourceValue,
			TargetField:       "/spec/replicas",
			TargetKind:        "Deployment",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       serviceAccountName,
			TargetField:       "/spec/template/spec/serviceAccountName",
			TargetKind:        "Deployment",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       instanceName,
			TargetField:       "/spec/selector/matchLabels" + instanceLabelPath,
			TargetKind:        "Deployment",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       instanceName,
			TargetField:       "/spec/template/metadata/labels" + instanceLabelPath,
			TargetKind:        "Deployment",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       serviceAccountName,
			TargetField:       "/subjects/0/name",
			TargetKind:        "RoleBinding",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       instanceNamespace,
			TargetField:       "/subjects/0/namespace",
			TargetKind:        "RoleBinding",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       instanceName,
			TargetField:       "/spec/selector/matchLabels" + instanceLabelPath,
			TargetKind:        "PodDisruptionBudget",
			CreateIfNotExists: true,
		},
		{
			SourceValue:       instanceName,
			TargetField:       "/spec/scaleTargetRef/name",
			TargetKind:        "HorizontalPodAutoscaler",
			CreateIfNotExists: true,
		},
	}
}

// getStorageSize extracts the storage size from the CR spec.
func getStorageSize(instance *llamav1alpha1.LlamaStackDistribution) string {
	if instance.Spec.Server.Storage != nil && instance.Spec.Server.Storage.Size != nil {
		return instance.Spec.Server.Storage.Size.String()
	}
	// Returning an empty string signals the field transformer to use the default value.
	return ""
}

// getServicePort returns the service port or nil if not specified.
func getServicePort(instance *llamav1alpha1.LlamaStackDistribution) any {
	if instance.Spec.Server.ContainerSpec.Port != 0 {
		return instance.Spec.Server.ContainerSpec.Port
	}
	// Returning nil signals the field transformer to use the default value.
	return nil
}

func isAutoscalingEnabled(instance *llamav1alpha1.LlamaStackDistribution) bool {
	if instance == nil || instance.Spec.Server.Autoscaling == nil {
		return false
	}

	return instance.Spec.Server.Autoscaling.MaxReplicas > 0
}

// ManifestContext provides the necessary context for complex resource rendering.
type ManifestContext struct {
	ResolvedImage           string
	ConfigMapHash           string
	CABundleHash            string
	ContainerSpec           map[string]any
	PodSpec                 map[string]any
	PodDisruptionBudgetSpec *policyv1.PodDisruptionBudgetSpec
	HPASpec                 *autoscalingv2.HorizontalPodAutoscalerSpec
}

// RenderManifestWithContext renders manifests and enhances the Deployment with complex specs.
func RenderManifestWithContext(
	fs filesys.FileSystem,
	manifestsPath string,
	ownerInstance *llamav1alpha1.LlamaStackDistribution,
	manifestCtx *ManifestContext,
) (*resmap.ResMap, error) {
	// First, render the base manifests
	resMap, err := RenderManifest(fs, manifestsPath, ownerInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to render base manifests: %w", err)
	}

	// If no manifest context provided, return base manifests
	if manifestCtx == nil {
		return resMap, nil
	}

	// Update the resources with the manifest context
	for _, res := range (*resMap).Resources() {
		switch res.GetKind() {
		case deploymentKind:
			if err := updateDeploymentSpec(res, manifestCtx); err != nil {
				return nil, fmt.Errorf("failed to update Deployment: %w", err)
			}
		case "PodDisruptionBudget":
			if err := updatePodDisruptionBudget(res, manifestCtx); err != nil {
				return nil, fmt.Errorf("failed to update PodDisruptionBudget: %w", err)
			}
		case "HorizontalPodAutoscaler":
			if err := updateHorizontalPodAutoscaler(res, manifestCtx); err != nil {
				return nil, fmt.Errorf("failed to update HorizontalPodAutoscaler: %w", err)
			}
		}
	}

	return resMap, nil
}

// updateDeploymentSpec updates the Deployment spec with the manifest context.
func updateDeploymentSpec(res *resource.Resource, manifestCtx *ManifestContext) error {
	// Parse the deployment YAML
	data, err := parseResourceYAML(res)
	if err != nil {
		return err
	}

	// Navigate to template spec
	templateSpec, err := getDeploymentTemplateSpec(data)
	if err != nil {
		return err
	}

	// Apply pod spec enhancements
	// Sort keys to ensure deterministic ordering and prevent spurious deployment updates
	if manifestCtx.PodSpec != nil {
		keys := make([]string, 0, len(manifestCtx.PodSpec))
		for key := range manifestCtx.PodSpec {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			templateSpec[key] = manifestCtx.PodSpec[key]
		}
	}

	// Add ConfigMap hash annotations
	if err := addConfigMapAnnotations(data, manifestCtx); err != nil {
		return err
	}

	// Update the resource with the manifest context
	return updateResourceFromData(res, data)
}

// parseResourceYAML parses a resource YAML into a map.
func parseResourceYAML(res *resource.Resource) (map[string]any, error) {
	yamlBytes, err := res.AsYAML()
	if err != nil {
		return nil, fmt.Errorf("failed to get YAML: %w", err)
	}

	var data map[string]any
	if unmarshalErr := yamlpkg.Unmarshal(yamlBytes, &data); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", unmarshalErr)
	}

	return data, nil
}

// getDeploymentTemplateSpec navigates to the deployment template spec.
func getDeploymentTemplateSpec(data map[string]any) (map[string]any, error) {
	spec, ok := data["spec"].(map[string]any)
	if !ok {
		return nil, errors.New("failed to find deployment spec")
	}

	template, ok := spec["template"].(map[string]any)
	if !ok {
		return nil, errors.New("failed to find deployment template")
	}

	templateSpec, ok := template["spec"].(map[string]any)
	if !ok {
		return nil, errors.New("failed to find deployment template spec")
	}

	return templateSpec, nil
}

// addConfigMapAnnotations adds ConfigMap hash annotations to the deployment template.
func addConfigMapAnnotations(data map[string]any, manifestCtx *ManifestContext) error {
	spec, ok := data["spec"].(map[string]any)
	if !ok {
		return errors.New("failed to find deployment spec in data")
	}

	template, ok := spec["template"].(map[string]any)
	if !ok {
		return errors.New("failed to find deployment template in spec")
	}

	templateMeta, ok := template["metadata"].(map[string]any)
	if !ok {
		templateMeta = make(map[string]any)
		template["metadata"] = templateMeta
	}

	annotations, ok := templateMeta["annotations"].(map[string]any)
	if !ok {
		annotations = make(map[string]any)
		templateMeta["annotations"] = annotations
	}

	if manifestCtx.ConfigMapHash != "" {
		annotations["configmap.hash/user-config"] = manifestCtx.ConfigMapHash
	}
	if manifestCtx.CABundleHash != "" {
		annotations["configmap.hash/ca-bundle"] = manifestCtx.CABundleHash
	}

	return nil
}

func updatePodDisruptionBudget(res *resource.Resource, manifestCtx *ManifestContext) error {
	if manifestCtx.PodDisruptionBudgetSpec == nil {
		return nil
	}
	data, err := parseResourceYAML(res)
	if err != nil {
		return err
	}
	spec, ok := data["spec"].(map[string]any)
	if !ok {
		return errors.New("failed to find PDB spec in data")
	}

	if manifestCtx.PodDisruptionBudgetSpec.MinAvailable != nil {
		spec["minAvailable"] = intOrStringToInterface(manifestCtx.PodDisruptionBudgetSpec.MinAvailable)
	} else {
		delete(spec, "minAvailable")
	}
	if manifestCtx.PodDisruptionBudgetSpec.MaxUnavailable != nil {
		spec["maxUnavailable"] = intOrStringToInterface(manifestCtx.PodDisruptionBudgetSpec.MaxUnavailable)
	} else {
		delete(spec, "maxUnavailable")
	}

	return updateResourceFromData(res, data)
}

func updateHorizontalPodAutoscaler(res *resource.Resource, manifestCtx *ManifestContext) error {
	if manifestCtx.HPASpec == nil {
		return nil
	}
	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(manifestCtx.HPASpec)
	if err != nil {
		return fmt.Errorf("failed to convert HPA spec: %w", err)
	}
	data, err := parseResourceYAML(res)
	if err != nil {
		return err
	}
	data["spec"] = specMap
	return updateResourceFromData(res, data)
}

func intOrStringToInterface(value *intstr.IntOrString) any {
	if value == nil {
		return nil
	}
	if value.Type == intstr.String {
		return value.StrVal
	}
	return value.IntValue()
}

// updateResourceFromData updates the resource with the modified data.
func updateResourceFromData(res *resource.Resource, data map[string]any) error {
	updatedJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal updated data: %w", err)
	}

	updatedYAML, err := yamlpkg.JSONToYAML(updatedJSON)
	if err != nil {
		return fmt.Errorf("failed to convert JSON to YAML: %w", err)
	}

	rf := resource.NewFactory(nil)
	newRes, err := rf.FromBytes(updatedYAML)
	if err != nil {
		return fmt.Errorf("failed to create resource from updated YAML: %w", err)
	}

	res.ResetRNode(newRes)
	return nil
}

func FilterExcludeKinds(resMap *resmap.ResMap, kindsToExclude []string) (*resmap.ResMap, error) {
	filteredResMap := resmap.New()
	for _, res := range (*resMap).Resources() {
		if !slices.Contains(kindsToExclude, res.GetKind()) {
			if err := filteredResMap.Append(res); err != nil {
				return nil, fmt.Errorf("failed to append resource while filtering %s/%s: %w", res.GetKind(), res.GetName(), err)
			}
		}
	}
	return &filteredResMap, nil
}

// hasVolume reports whether a volume with the given name exists in the slice.
func hasVolume(volumes []corev1.Volume, name string) bool {
	for _, vol := range volumes {
		if vol.Name == name {
			return true
		}
	}
	return false
}

// hasLegacyCABundleVolumes detects if a deployment has legacy CA bundle volumes
// from the old operator that used emptyDir + ConfigMap pattern.
func hasLegacyCABundleVolumes(ctx context.Context, deployment *unstructured.Unstructured) bool {
	logger := log.FromContext(ctx)

	volumes, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "volumes")
	if err != nil || !found {
		return false
	}

	for _, vol := range volumes {
		volumeMap, ok := vol.(map[string]interface{})
		if !ok {
			continue
		}

		volumeName, _, _ := unstructured.NestedString(volumeMap, "name")

		// Legacy pattern: volume named "ca-bundle" with emptyDir
		if volumeName == "ca-bundle" {
			if _, hasEmptyDir := volumeMap["emptyDir"]; hasEmptyDir {
				logger.V(1).Info("Found legacy ca-bundle emptyDir volume",
					"deployment", deployment.GetName(),
					"namespace", deployment.GetNamespace())
				return true
			}
		}

		// Legacy pattern: volume named "ca-bundle-source" (ConfigMap source)
		if volumeName == "ca-bundle-source" {
			logger.V(1).Info("Found legacy ca-bundle-source volume",
				"deployment", deployment.GetName(),
				"namespace", deployment.GetNamespace())
			return true
		}
	}

	return false
}

// hasStaleUserConfigVolume returns true when the existing Deployment has a "user-config"
// volume that is absent from the desired Deployment spec. This happens when
// spec.server.userConfig is removed from the LLSD resource: the volume persists because
// it was applied via cli.Create (no SSA field manager tracking), so a subsequent SSA patch
// cannot remove it. Using cli.Update instead performs a full spec replacement.
func hasStaleUserConfigVolume(desired, existing *appsv1.Deployment) bool {
	return hasVolume(existing.Spec.Template.Spec.Volumes, "user-config") &&
		!hasVolume(desired.Spec.Template.Spec.Volumes, "user-config")
}

// deploymentNeedsFullReplacement returns a non-empty reason string when the Deployment
// must be updated via cli.Update (full replacement) instead of SSA. This is necessary
// when volumes exist in the live Deployment that SSA cannot remove because they were
// created by a different field manager (e.g. the initial cli.Create).
func deploymentNeedsFullReplacement(ctx context.Context, desired, existing *unstructured.Unstructured) string {
	logger := log.FromContext(ctx)

	if hasLegacyCABundleVolumes(ctx, existing) {
		return "legacy CA bundle volumes detected"
	}

	var existingDep, desiredDep appsv1.Deployment
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(existing.Object, &existingDep); err != nil {
		logger.Error(err, "failed to convert existing Deployment, skipping stale-volume check")
		return ""
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(desired.Object, &desiredDep); err != nil {
		logger.Error(err, "failed to convert desired Deployment, skipping stale-volume check")
		return ""
	}
	if hasStaleUserConfigVolume(&desiredDep, &existingDep) {
		return "stale user-config volume detected"
	}
	return ""
}

// CheckClusterRoleExists checks if a RoleBinding should be skipped due to missing SCC ClusterRole.
func CheckClusterRoleExists(ctx context.Context, cli client.Client, crb *unstructured.Unstructured) (bool, error) {
	roleRef, found, _ := unstructured.NestedMap(crb.Object, "roleRef")
	if !found {
		return false, nil // No roleRef, don't skip
	}

	roleName, _, _ := unstructured.NestedString(roleRef, "name")
	if roleName == "" {
		return false, nil // Empty roleName, don't skip
	}

	// Check if the referenced ClusterRole exists
	clusterRole := &unstructured.Unstructured{}
	clusterRole.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "rbac.authorization.k8s.io",
		Version: "v1",
		Kind:    "ClusterRole",
	})
	clusterRole.SetName(roleName)

	err := cli.Get(ctx, client.ObjectKey{Name: roleName}, clusterRole)
	if err != nil && k8serr.IsNotFound(err) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	return false, nil
}
