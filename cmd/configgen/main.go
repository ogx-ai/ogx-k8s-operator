/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main implements a CLI tool for using the operator's config
// generation pipeline outside of Kubernetes. Given an OGXServer CR, it:
//
//  1. Optionally validates the CR (webhook, schema, and CEL validation).
//  2. Resolves a base config from -base, or via OCI labels on distribution.image.
//  3. Expands providers/resources/storage from the spec.
//  4. Outputs the metadata, env var mappings, and config.yaml that the OGX server would receive at runtime.
//
// Usage:
//
//	configgen <ogxserver.yaml> [-base <config.yaml>] [-crd-path <dir>] [-distributions-path <file>] [-output-config] [-validate]
//
// Notes:
//   - spec.baseConfig cannot be dereferenced by the CLI; pass that file with -base.
//   - spec.distribution.name-only CRs also need -base, since image resolution happens in the operator.
//   - -validate uses distributions.json to validate spec.distribution.name.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func main() {
	opts := parseFlags()

	generated, err := run(opts)
	if err != nil {
		log.Fatal(err)
	}

	if opts.outputConfig {
		fmt.Print(generated.ConfigYAML)
	} else {
		printFullOutput(generated)
	}
}

func run(opts options) (*config.GeneratedConfig, error) {
	server, err := loadCR(opts.crPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load CR: %w", err)
	}

	if server.HasOverrideConfig() {
		return nil, errors.New("failed to generate config: CR has overrideConfig set; the operator would skip config generation and use the override ConfigMap directly")
	}
	if !server.HasDeclarativeConfig() {
		return nil, errors.New("failed to generate config: CR has no declarative config fields (providers, resources, storage, or disabledAPIs); nothing to generate")
	}

	if opts.validate {
		if validateErr := validateCR(server, opts.crdPath, opts.distributionsPath); validateErr != nil {
			return nil, fmt.Errorf("failed to validate CR:\n%w", validateErr)
		}
	}

	baseConfigData, err := resolveBaseConfig(opts.basePath, &server.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base config: %w", err)
	}

	if err := config.ValidateSecretRefEnvVarNames(&server.Spec); err != nil {
		return nil, fmt.Errorf("failed to validate secret ref env var names: %w", err)
	}

	return config.GenerateConfig(&server.Spec, baseConfigData)
}

type options struct {
	crPath            string
	basePath          string
	crdPath           string
	distributionsPath string
	outputConfig      bool
	validate          bool
}

func parseFlags() options {
	basePath := flag.String("base", "", "path to base config.yaml (omit to resolve via OCI labels on spec.distribution.image)")
	crdPath := flag.String("crd-path", "config/crd/bases", "path to CRD bases directory (used with -validate)")
	distributionsPath := flag.String("distributions-path", "distributions.json", "path to distributions.json for validating spec.distribution.name")
	outputConfig := flag.Bool("output-config", false, "print only the generated config YAML (no metadata or env vars)")
	validate := flag.Bool("validate", false, "validate the CR against the CRD schema, CEL rules, and webhook logic (requires KUBEBUILDER_ASSETS)")
	flag.Parse()

	crPath := flag.Arg(0)
	if crPath == "" {
		log.Fatal("usage: configgen <ogxserver.yaml> [-base <config.yaml>] [-crd-path <dir>] [-distributions-path <file>] [-output-config] [-validate]")
	}

	return options{
		crPath:            crPath,
		basePath:          *basePath,
		crdPath:           *crdPath,
		distributionsPath: *distributionsPath,
		outputConfig:      *outputConfig,
		validate:          *validate,
	}
}

func loadCR(path string) (*ogxiov1beta1.OGXServer, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	s := runtime.NewScheme()
	utilruntime.Must(ogxiov1beta1.AddToScheme(s))
	codecs := serializer.NewCodecFactory(s)
	decoder := codecs.UniversalDeserializer()

	obj, _, err := decoder.Decode(data, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CR: %w", err)
	}

	server, ok := obj.(*ogxiov1beta1.OGXServer)
	if !ok {
		return nil, fmt.Errorf("failed to cast decoded object: expected OGXServer, got %T", obj)
	}

	return server, nil
}

func validateCR(server *ogxiov1beta1.OGXServer, crdPath, distributionsPath string) error {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		return errors.New("failed to find KUBEBUILDER_ASSETS: not set; run: make envtest && export KUBEBUILDER_ASSETS=$(bin/setup-envtest use 1.31.0 --bin-dir bin -p path)")
	}

	if info, err := os.Stat(crdPath); err != nil || !info.IsDir() {
		return fmt.Errorf("failed to find CRD path %q: does not exist or is not a directory; use -crd-path to specify", crdPath)
	}

	var errs []string

	// CRD schema + CEL validation via envtest
	if err := validateWithEnvTest(server, crdPath, assets); err != nil {
		errs = append(errs, fmt.Sprintf("CRD/CEL: %s", err))
	}

	// Webhook validation (in-process, no server needed)
	knownDistNames, err := loadKnownDistributionNames(server, distributionsPath)
	if err != nil {
		errs = append(errs, fmt.Sprintf("webhook: %s", err))
	}
	validator := &ogxiov1beta1.OGXServerValidator{
		KnownDistributionNames: knownDistNames,
	}
	if _, err := validator.ValidateCreate(context.Background(), server); err != nil {
		errs = append(errs, fmt.Sprintf("webhook: %s", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to validate CR:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

func validateWithEnvTest(server *ogxiov1beta1.OGXServer, crdPath, assets string) error {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assets,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		return fmt.Errorf("failed to start envtest: %w", err)
	}
	defer func() { _ = testEnv.Stop() }()

	utilruntime.Must(ogxiov1beta1.AddToScheme(scheme.Scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create envtest client: %w", err)
	}

	ctx := context.Background()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "configgen-validate"},
	}
	if err := k8sClient.Create(ctx, ns); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	cr := server.DeepCopy()
	cr.Namespace = ns.Name
	if cr.Name == "" {
		cr.Name = "configgen-validate"
	}

	if err := k8sClient.Create(ctx, cr, client.DryRunAll); err != nil {
		return err
	}

	return nil
}

func printFullOutput(g *config.GeneratedConfig) {
	fmt.Println("--- config.yaml ---")
	fmt.Print(g.ConfigYAML)

	fmt.Printf("\n--- metadata ---\n")
	fmt.Printf("content-hash:   %s\n", g.ContentHash)
	fmt.Printf("config-version: %d\n", g.ConfigVersion)
	fmt.Printf("config-version-defaulted: %t\n", g.ConfigVersionDefaulted)
	fmt.Printf("providers:      %d\n", g.ProviderCount)
	fmt.Printf("resources:      %d\n", g.ResourceCount)

	if len(g.EnvVars) > 0 {
		fmt.Println("\n--- env vars ---")
		for _, e := range g.EnvVars {
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
				fmt.Printf("%s -> %s/%s\n", e.Name, e.ValueFrom.SecretKeyRef.Name, e.ValueFrom.SecretKeyRef.Key)
			}
		}
	}
}

func loadKnownDistributionNames(server *ogxiov1beta1.OGXServer, distributionsPath string) ([]string, error) {
	if server.Spec.Distribution.Name == "" {
		return nil, nil
	}

	data, err := readFile(distributionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read distributions file %q: %w", distributionsPath, err)
	}

	var distributionImages map[string]string
	if err := json.Unmarshal(data, &distributionImages); err != nil {
		return nil, fmt.Errorf("failed to parse distributions file %q: %w", distributionsPath, err)
	}

	names := make([]string, 0, len(distributionImages))
	for name := range distributionImages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func resolveBaseConfig(basePath string, spec *ogxiov1beta1.OGXServerSpec) ([]byte, error) {
	if basePath != "" {
		return readFile(basePath)
	}
	if spec.BaseConfig != nil {
		return nil, errors.New("configgen cannot read spec.baseConfig from Kubernetes; pass -base with the ConfigMap's config.yaml contents")
	}
	if spec.Distribution.Image == "" {
		return nil, errors.New("configgen requires spec.distribution.image or -base because named distributions are resolved by the operator")
	}
	resolver := config.NewDefaultConfigResolver(config.NewOCILabelFetcher())
	return resolver.Resolve(spec.Distribution.Image, spec.Distribution.Name)
}

func readFile(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path) //nolint:gosec // CLI tool intentionally reads user-provided file paths
}
