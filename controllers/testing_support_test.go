package controllers_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	controllers "github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testTimeout  = 5 * time.Second
	testInterval = 100 * time.Millisecond

	testImage             = "ogx/ogx-ollama:1.0"
	testOperatorNamespace = "default"
	testStorageVolumeName = "ogx-storage"
	testInstanceName      = "test-instance"
)

// OGXServerBuilder - Builder pattern for test instances of operator custom resource.
type OGXServerBuilder struct {
	instance *ogxiov1beta1.OGXServer
}

func NewOGXServerBuilder() *OGXServerBuilder {
	return &OGXServerBuilder{
		instance: &ogxiov1beta1.OGXServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testInstanceName,
				Namespace: "default",
			},
			Spec: ogxiov1beta1.OGXServerSpec{
				Distribution: ogxiov1beta1.DistributionSpec{
					Name: "starter",
				},
			},
		},
	}
}

func (b *OGXServerBuilder) WithName(name string) *OGXServerBuilder {
	b.instance.Name = name
	return b
}

func (b *OGXServerBuilder) WithNamespace(namespace string) *OGXServerBuilder {
	b.instance.Namespace = namespace
	return b
}

func (b *OGXServerBuilder) WithPort(port int32) *OGXServerBuilder {
	if b.instance.Spec.Network == nil {
		b.instance.Spec.Network = &ogxiov1beta1.NetworkSpec{}
	}
	b.instance.Spec.Network.Port = port
	return b
}

func (b *OGXServerBuilder) WithReplicas(replicas int32) *OGXServerBuilder {
	if b.instance.Spec.Workload == nil {
		b.instance.Spec.Workload = &ogxiov1beta1.WorkloadSpec{}
	}
	b.instance.Spec.Workload.Replicas = &replicas
	return b
}

func (b *OGXServerBuilder) WithStorage(storage *ogxiov1beta1.PVCStorageSpec) *OGXServerBuilder {
	if b.instance.Spec.Workload == nil {
		b.instance.Spec.Workload = &ogxiov1beta1.WorkloadSpec{}
	}
	b.instance.Spec.Workload.Storage = storage
	return b
}

func (b *OGXServerBuilder) WithDistribution(distributionName string) *OGXServerBuilder {
	b.instance.Spec.Distribution.Name = distributionName
	return b
}

func (b *OGXServerBuilder) WithResources(resources corev1.ResourceRequirements) *OGXServerBuilder {
	if b.instance.Spec.Workload == nil {
		b.instance.Spec.Workload = &ogxiov1beta1.WorkloadSpec{}
	}
	b.instance.Spec.Workload.Resources = &resources
	return b
}

func (b *OGXServerBuilder) WithServiceAccountName(serviceAccountName string) *OGXServerBuilder {
	if b.instance.Spec.Workload == nil {
		b.instance.Spec.Workload = &ogxiov1beta1.WorkloadSpec{}
	}
	if b.instance.Spec.Workload.Overrides == nil {
		b.instance.Spec.Workload.Overrides = &ogxiov1beta1.WorkloadOverrides{}
	}
	b.instance.Spec.Workload.Overrides.ServiceAccountName = serviceAccountName
	return b
}

func (b *OGXServerBuilder) WithOverrideConfig(configMapName string) *OGXServerBuilder {
	b.instance.Spec.OverrideConfig = &ogxiov1beta1.OverrideConfigSpec{
		ConfigMapName: configMapName,
	}
	return b
}

func (b *OGXServerBuilder) WithCABundle(configMapName string, configMapKeys []string) *OGXServerBuilder {
	b.instance.Spec.CABundle = &ogxiov1beta1.CABundleConfig{
		ConfigMapName: configMapName,
		ConfigMapKeys: configMapKeys,
	}
	return b
}

func (b *OGXServerBuilder) Build() *ogxiov1beta1.OGXServer {
	return b.instance.DeepCopy()
}

func DefaultTestStorage() *ogxiov1beta1.PVCStorageSpec {
	return &ogxiov1beta1.PVCStorageSpec{}
}

func CustomTestStorage(size string, mountPath string) *ogxiov1beta1.PVCStorageSpec {
	sizeQuantity := resource.MustParse(size)
	return &ogxiov1beta1.PVCStorageSpec{
		Size:      &sizeQuantity,
		MountPath: mountPath,
	}
}

func AssertDeploymentHasCorrectImage(t *testing.T, deployment *appsv1.Deployment, expectedImage string) {
	t.Helper()
	require.NotEmpty(t, deployment.Spec.Template.Spec.Containers,
		"deployment should have at least one container")

	actualImage := deployment.Spec.Template.Spec.Containers[0].Image
	require.Equal(t, expectedImage, actualImage,
		"deployment container should use the correct image")
}

func AssertDeploymentHasPort(t *testing.T, deployment *appsv1.Deployment, expectedPort int32) {
	t.Helper()
	require.NotEmpty(t, deployment.Spec.Template.Spec.Containers,
		"deployment should have at least one container")

	container := deployment.Spec.Template.Spec.Containers[0]
	require.NotEmpty(t, container.Ports, "container should expose at least one port")

	actualPort := container.Ports[0].ContainerPort
	require.Equal(t, expectedPort, actualPort,
		"container should expose port %d", expectedPort)
}

func AssertDeploymentUsesEmptyDirStorage(t *testing.T, deployment *appsv1.Deployment) {
	t.Helper()
	volume := findVolumeByName(t, deployment, testStorageVolumeName)
	require.NotNil(t, volume.EmptyDir, "deployment should use emptyDir storage")
	require.Nil(t, volume.PersistentVolumeClaim, "deployment should not use PVC storage")
}

func AssertDeploymentUsesPVCStorage(t *testing.T, deployment *appsv1.Deployment, expectedPVCName string) {
	t.Helper()
	volume := findVolumeByName(t, deployment, testStorageVolumeName)
	require.NotNil(t, volume.PersistentVolumeClaim, "deployment should use PVC storage")
	require.Nil(t, volume.EmptyDir, "deployment should not use emptyDir storage")
	require.Equal(t, expectedPVCName, volume.PersistentVolumeClaim.ClaimName,
		"deployment should reference the correct PVC")
}

func AssertDeploymentHasVolumeMount(t *testing.T, deployment *appsv1.Deployment, expectedMountPath string) {
	t.Helper()
	require.NotEmpty(t, deployment.Spec.Template.Spec.Containers,
		"deployment should have at least one container")

	container := deployment.Spec.Template.Spec.Containers[0]
	mount := findVolumeMountByName(t, container, testStorageVolumeName)
	require.Equal(t, expectedMountPath, mount.MountPath,
		"volume should be mounted at the correct path")
}

func AssertPVCExists(t *testing.T, client client.Client, namespace, name string) *corev1.PersistentVolumeClaim {
	t.Helper()
	pvc := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Name: name, Namespace: namespace}

	require.Eventually(t, func() bool {
		return client.Get(t.Context(), key, pvc) == nil
	}, testTimeout, testInterval, "PVC %s should exist in namespace %s", name, namespace)

	return pvc
}

func AssertServiceExposesDeployment(t *testing.T, service *corev1.Service, deployment *appsv1.Deployment) {
	t.Helper()

	require.Equal(t, service.Spec.Selector, deployment.Spec.Template.Labels,
		"service selector should match deployment pod labels for traffic routing")

	require.NotEmpty(t, service.Spec.Ports, "service should expose at least one port")
	require.NotEmpty(t, deployment.Spec.Template.Spec.Containers, "deployment should have at least one container")

	servicePort := service.Spec.Ports[0]
	containerPort := deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort
	require.Equal(t, servicePort.TargetPort.IntVal, containerPort,
		"service target port should route to deployment container port")
}

func AssertNetworkPolicyProtectsDeployment(t *testing.T, networkPolicy *networkingv1.NetworkPolicy, deployment *appsv1.Deployment) {
	t.Helper()

	require.Equal(t, deployment.Spec.Template.Labels, networkPolicy.Spec.PodSelector.MatchLabels,
		"network policy should protect the same pods as deployment")

	require.NotEmpty(t, deployment.Spec.Template.Spec.Containers, "deployment should have containers")
	require.NotEmpty(t, networkPolicy.Spec.Ingress, "network policy should have ingress rules")
	require.NotEmpty(t, networkPolicy.Spec.Ingress[0].Ports, "network policy should allow specific ports")

	containerPort := deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort
	policyPort := networkPolicy.Spec.Ingress[0].Ports[0].Port.IntVal
	require.Equal(t, containerPort, policyPort,
		"network policy should allow traffic on deployment container port")
}

func AssertResourceOwnedByInstance(t *testing.T, resource metav1.Object, instance *ogxiov1beta1.OGXServer) {
	t.Helper()

	ownerRefs := resource.GetOwnerReferences()
	require.Len(t, ownerRefs, 1, "resource should have exactly one owner reference")
	require.Equal(t, instance.GetUID(), ownerRefs[0].UID,
		"resource should be owned by the OGXServer instance for garbage collection")
}

func AssertClusterRoleBindingLinksServiceAccount(t *testing.T, crb *rbacv1.ClusterRoleBinding, serviceAccount *corev1.ServiceAccount) {
	t.Helper()

	require.NotEmpty(t, crb.Subjects, "cluster role binding should have subjects")

	found := false
	for _, subject := range crb.Subjects {
		if subject.Kind == "ServiceAccount" &&
			subject.Name == serviceAccount.Name &&
			subject.Namespace == serviceAccount.Namespace {
			found = true
			break
		}
	}
	require.True(t, found,
		"cluster role binding should grant permissions to the service account %s/%s",
		serviceAccount.Namespace, serviceAccount.Name)
}

func AssertDeploymentUsesServiceAccount(t *testing.T, deployment *appsv1.Deployment, serviceAccount *corev1.ServiceAccount) {
	t.Helper()

	require.Equal(t, serviceAccount.Name, deployment.Spec.Template.Spec.ServiceAccountName,
		"deployment pods should use the service account for proper permissions")
}

func AssertPVCHasSize(t *testing.T, pvc *corev1.PersistentVolumeClaim, expectedSize string) {
	t.Helper()
	storageRequest, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	require.True(t, ok, "PVC should have storage request")

	expectedQuantity := resource.MustParse(expectedSize)
	require.True(t, expectedQuantity.Equal(storageRequest),
		"PVC should request %s storage, got %s", expectedSize, storageRequest.String())
}

func AssertServicePortMatches(t *testing.T, service *corev1.Service, expectedPort corev1.ServicePort) {
	t.Helper()
	require.Len(t, service.Spec.Ports, 1, "Service should have exactly one port")
	require.Equal(t, expectedPort, service.Spec.Ports[0], "Service port should match expected")
}

func AssertServiceAndDeploymentPortsAlign(t *testing.T, service *corev1.Service, deployment *appsv1.Deployment) {
	t.Helper()
	require.Len(t, service.Spec.Ports, 1, "Service should have exactly one port")
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1, "Deployment should have exactly one container")
	require.Len(t, deployment.Spec.Template.Spec.Containers[0].Ports, 1, "Container should have exactly one port")

	serviceTargetPort := service.Spec.Ports[0].TargetPort.IntVal
	containerPort := deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort
	require.Equal(t, serviceTargetPort, containerPort, "Service target port should match deployment container port")
}

func AssertServiceSelectorMatches(t *testing.T, service *corev1.Service, expectedSelector map[string]string) {
	t.Helper()
	require.Equal(t, expectedSelector, service.Spec.Selector, "Service selector should match expected")
}

func AssertServiceAndDeploymentSelectorsAlign(t *testing.T, service *corev1.Service, deployment *appsv1.Deployment) {
	t.Helper()
	require.Equal(t, service.Spec.Selector, deployment.Spec.Template.Labels, "Service selector should match deployment pod labels")
}

func AssertNetworkPolicyTargetsDeploymentPods(t *testing.T, networkPolicy *networkingv1.NetworkPolicy, deployment *appsv1.Deployment) {
	t.Helper()
	require.Equal(t, deployment.Spec.Template.Labels, networkPolicy.Spec.PodSelector.MatchLabels,
		"NetworkPolicy should target same pods as deployment")
}

func hasMatchingIngressRule(
	t *testing.T,
	policy *networkingv1.NetworkPolicy,
	port int32,
	peerPredicate func(peer networkingv1.NetworkPolicyPeer) bool,
) bool {
	t.Helper()
	for _, rule := range policy.Spec.Ingress {
		if !slices.ContainsFunc(rule.From, peerPredicate) {
			continue
		}

		portMatches := slices.ContainsFunc(rule.Ports, func(p networkingv1.NetworkPolicyPort) bool {
			return p.Port != nil && p.Port.IntVal == port
		})

		if portMatches {
			return true
		}
	}

	return false
}

func AssertNetworkPolicyAllowsDeploymentPort(t *testing.T, networkPolicy *networkingv1.NetworkPolicy, deployment *appsv1.Deployment, operatorNamespace string) {
	t.Helper()
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1, "Deployment should have exactly one container")
	require.Len(t, deployment.Spec.Template.Spec.Containers[0].Ports, 1, "Container should have exactly one port")
	containerPort := deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort

	sameNamespacePredicate := func(peer networkingv1.NetworkPolicyPeer) bool {
		return peer.PodSelector != nil && len(peer.PodSelector.MatchLabels) == 0 && peer.NamespaceSelector == nil
	}
	require.True(t,
		hasMatchingIngressRule(t, networkPolicy, containerPort, sameNamespacePredicate),
		"NetworkPolicy is missing a rule to allow traffic from all pods in the same namespace on port %d", containerPort)

	operatorPredicate := func(peer networkingv1.NetworkPolicyPeer) bool {
		return peer.NamespaceSelector != nil && peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == operatorNamespace
	}
	require.True(t,
		hasMatchingIngressRule(t, networkPolicy, containerPort, operatorPredicate),
		"NetworkPolicy is missing a rule to allow traffic from the operator in namespace '%s' on port %d", operatorNamespace, containerPort)
}

func AssertNetworkPolicyIsIngressOnly(t *testing.T, networkPolicy *networkingv1.NetworkPolicy) {
	t.Helper()
	expectedPolicyTypes := []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}
	require.Equal(t, expectedPolicyTypes, networkPolicy.Spec.PolicyTypes, "NetworkPolicy should be ingress-only")
}

func AssertNetworkPolicyAbsent(t *testing.T, c client.Client, key types.NamespacedName) {
	t.Helper()
	require.Eventually(t, func() bool {
		var np networkingv1.NetworkPolicy
		err := c.Get(t.Context(), key, &np)
		return apierrors.IsNotFound(err)
	}, testTimeout, testInterval, "NetworkPolicy %s/%s should not exist", key.Namespace, key.Name)
}

func AssertServiceAccountDeploymentAlign(t *testing.T, deployment *appsv1.Deployment, serviceAccount *corev1.ServiceAccount) {
	t.Helper()
	require.Equal(t, serviceAccount.Name, deployment.Spec.Template.Spec.ServiceAccountName,
		"Deployment should use the created ServiceAccount for pod permissions")
}

func ReconcileOGXServer(t *testing.T, instance *ogxiov1beta1.OGXServer) {
	t.Helper()
	reconciler := createTestReconciler()
	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	})
	require.NoError(t, err, "reconciliation should succeed")
}

func ResourceTestName(instanceName, suffix string) string {
	return instanceName + suffix
}

func createTestReconciler() *controllers.OGXServerReconciler {
	clusterInfo := &cluster.ClusterInfo{
		OperatorNamespace: testOperatorNamespace,
		DistributionImages: map[string]string{
			"starter": testImage,
		},
	}
	return controllers.NewTestReconciler(k8sClient, scheme.Scheme, clusterInfo, &http.Client{})
}

func findVolumeByName(t *testing.T, deployment *appsv1.Deployment, volumeName string) *corev1.Volume {
	t.Helper()
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == volumeName {
			return &volume
		}
	}
	require.Fail(t, "volume not found", "deployment should have volume named %s", volumeName)
	return nil
}

func findVolumeMountByName(t *testing.T, container corev1.Container, volumeName string) *corev1.VolumeMount {
	t.Helper()
	for _, mount := range container.VolumeMounts {
		if mount.Name == volumeName {
			return &mount
		}
	}
	require.Fail(t, "volume mount not found", "container should have volume mount named %s", volumeName)
	return nil
}

func waitForResource(t *testing.T, client client.Client, namespace, name string, resource client.Object) {
	t.Helper()
	key := types.NamespacedName{Name: name, Namespace: namespace}
	waitForResourceWithKey(t, client, key, resource)
}

func waitForResourceWithKey(t *testing.T, client client.Client, key types.NamespacedName, resource client.Object) {
	t.Helper()
	waitForResourceWithKeyAndCondition(t, client, key, resource, nil, fmt.Sprintf("timed out waiting for %T %s to be available", resource, key))
}

func waitForResourceWithKeyAndCondition(t *testing.T, client client.Client, key types.NamespacedName, resource client.Object, condition func() bool, message string) {
	t.Helper()
	require.Eventually(t, func() bool {
		err := client.Get(t.Context(), key, resource)
		if err != nil {
			return false
		}
		if condition == nil {
			return true
		}
		return condition()
	}, testTimeout, testInterval, message)
}

func createTestNamespace(t *testing.T, namePrefix string) *corev1.Namespace {
	t.Helper()
	testenvNamespaceCounter++
	nsName := fmt.Sprintf("%s-%d", namePrefix, testenvNamespaceCounter)
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), namespace))

	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), namespace); err != nil {
			t.Logf("Failed to delete test namespace %s: %v", namespace.Name, err)
		}
	})
	return namespace
}

func loadTestCertificate(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "Failed to generate private key")

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err, "Failed to generate serial number")

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Country:            []string{"US"},
			Province:           []string{"California"},
			Locality:           []string{"Los Angeles"},
			Organization:       []string{"Test Corp"},
			OrganizationalUnit: []string{"Testing"},
			CommonName:         "test-ca",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err, "Failed to create certificate")

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	require.NotNil(t, certPEM, "Failed to encode certificate to PEM")

	return string(certPEM)
}
