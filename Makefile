# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.0.1

# LLAMASTACK_VERSION defines the version of LlamaStack distributions to use
LLAMASTACK_VERSION ?= latest

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# llamastack.io/llama-stack-k8s-operator-bundle:$VERSION and llamastack.io/llama-stack-k8s-operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= quay.io/llamastack/llama-stack-k8s-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.39.2
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

# Image URL to use all building/pushing image targets
IMG_TAG ?= latest
IMG ?= $(IMAGE_TAG_BASE):$(IMG_TAG)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker/Podman which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= podman

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# E2E tests additional flags
# See README.md, default go test timeout 10m
E2E_TEST_FLAGS = -timeout 30m

# Test configuration
TEST_PKGS ?= $$(go list ./... | grep -v /e2e)
TEST_FLAGS ?= -coverprofile cover.out

# Include local.mk if it exists (for custom overrides)
-include local.mk

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

GOLANGCI_TMP_FILE = .golangci.mktmp.yml

.PHONY: fmt
fmt: golangci-lint yq ## Formats code and imports.
	go fmt ./...
	$(YQ) e '.version = "2" | .formatters.enable = ["gci"] | .formatters.settings.gci = .linters.settings.gci' .golangci.yml > $(GOLANGCI_TMP_FILE)
	$(GOLANGCI_LINT) fmt --config=$(GOLANGCI_TMP_FILE)

.PHONY: clean
clean: ## Remove temporary files, caches, and downloaded tools
	@# Clean golangci-lint cache only if golangci-lint is available
	@if command -v $(GOLANGCI_LINT) >/dev/null 2>&1; then \
		$(GOLANGCI_LINT) cache clean; \
	fi
	rm -f $(GOLANGCI_TMP_FILE) Dockerfile.cross cover.out
	rm -rf $(LOCALBIN)

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests. Use TEST_PKGS and TEST_FLAGS to customize.
	GOTOOLCHAIN=go1.25.8+auto KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $(TEST_PKGS) $(TEST_FLAGS)

.PHONY: test-e2e
test-e2e: ## Run e2e tests
	./hack/deploy-quickstart.sh # Deploy Ollama for e2e tests
	go test -v ./tests/e2e/ -run ^TestE2E -v ${E2E_TEST_FLAGS}

GOLANGCI_LINT_TIMEOUT ?= 5m0s
.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run --timeout=$(GOLANGCI_LINT_TIMEOUT)

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint fix against code.
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: image-build
image-build: ## Build image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: image-push
image-push: ## Push image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make image-buildx IMG=myregistry/myoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
# Note: The Dockerfile uses --platform=$BUILDPLATFORM for native cross-compilation, avoiding QEMU emulation
# for Go builds which dramatically improves reliability and speed.
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: image-buildx
image-buildx: ## Build and push multi-arch image with the manager (uses native cross-compilation)
	@echo "Building multi-arch image for platforms: $(PLATFORMS)"
ifeq ($(CONTAINER_TOOL),docker)
	- $(CONTAINER_TOOL) buildx create --name x-builder 2>/dev/null || true
	$(CONTAINER_TOOL) buildx use x-builder
	$(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} .
else
	# Podman: Use manifest-based multi-arch build
	$(CONTAINER_TOOL) manifest rm ${IMG} 2>/dev/null || true
	$(CONTAINER_TOOL) manifest create ${IMG}
	@for platform in $$(echo $(PLATFORMS) | tr ',' ' '); do \
		echo "Building for $$platform..."; \
		$(CONTAINER_TOOL) build --platform $$platform --manifest ${IMG} . ; \
	done
	$(CONTAINER_TOOL) manifest push ${IMG}
endif
	@echo "Successfully pushed multi-arch image: ${IMG}"

.PHONY: image-buildx-build
image-buildx-build: ## Build multi-arch image with the manager without pushing (uses native cross-compilation)
	@echo "Building multi-arch image for platforms: $(PLATFORMS)"
ifeq ($(CONTAINER_TOOL),docker)
	- $(CONTAINER_TOOL) buildx create --name x-builder 2>/dev/null || true
	$(CONTAINER_TOOL) buildx use x-builder
	@mkdir -p /tmp/buildx-output
	$(CONTAINER_TOOL) buildx build --output type=local,dest=/tmp/buildx-output --platform=$(PLATFORMS) --tag ${IMG} .
	@rm -rf /tmp/buildx-output
else
	# Podman: Use manifest-based multi-arch build (build only, no push)
	$(CONTAINER_TOOL) manifest rm ${IMG} 2>/dev/null || true
	$(CONTAINER_TOOL) manifest create ${IMG}
	@for platform in $$(echo $(PLATFORMS) | tr ',' ' '); do \
		echo "Building for $$platform..."; \
		$(CONTAINER_TOOL) build --platform $$platform --manifest ${IMG} . ; \
	done
endif
	@echo "Successfully built multi-arch image: ${IMG}"

# PLATFORM defines the target platform for single-arch image builds used by CI matrix jobs.
# Each architecture is built natively on its own runner (e.g., ARM64 on ARM64 runner),
# ensuring CGO_ENABLED=1 with full OpenSSL FIPS support for all architectures.
PLATFORM ?= linux/amd64

.PHONY: image-build-push-single
image-build-push-single: ## Build and push a single-arch image (set PLATFORM and IMG)
	@echo "Building and pushing single-arch image for platform: $(PLATFORM)"
ifeq ($(CONTAINER_TOOL),docker)
	- $(CONTAINER_TOOL) buildx create --name x-builder 2>/dev/null || true
	$(CONTAINER_TOOL) buildx use x-builder
	$(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORM) --tag ${IMG} .
else
	$(CONTAINER_TOOL) build --platform $(PLATFORM) -t ${IMG} .
	$(CONTAINER_TOOL) push ${IMG}
endif
	@echo "Successfully pushed single-arch image: ${IMG} ($(PLATFORM))"

# ARCH_IMGS is a space-separated list of per-architecture image references
# used to create a multi-arch manifest (e.g., "myregistry/myimage:v1-amd64 myregistry/myimage:v1-arm64").
ARCH_IMGS ?=

.PHONY: image-create-manifest
image-create-manifest: ## Create and push a multi-arch manifest from per-arch images (set IMG and ARCH_IMGS)
	@echo "Creating multi-arch manifest: ${IMG}"
ifeq ($(CONTAINER_TOOL),docker)
	- $(CONTAINER_TOOL) buildx create --name x-builder 2>/dev/null || true
	$(CONTAINER_TOOL) buildx use x-builder
	$(CONTAINER_TOOL) buildx imagetools create -t ${IMG} $(ARCH_IMGS)
else
	$(CONTAINER_TOOL) manifest rm ${IMG} 2>/dev/null || true
	$(CONTAINER_TOOL) manifest create ${IMG} $(ARCH_IMGS)
	$(CONTAINER_TOOL) manifest push ${IMG}
endif
	@echo "Successfully pushed multi-arch manifest: ${IMG}"

.PHONY: image-build-arm
image-build-arm: ## Build ARM64 image with the manager
	$(CONTAINER_TOOL) build --platform linux/arm64 -t ${IMG} .

# Legacy docker-buildx target (deprecated, use image-buildx instead)
.PHONY: docker-buildx
docker-buildx: image-buildx ## Deprecated: use image-buildx instead

# Installer directory is updated to `release` from operator-sdk default `dist` directory.
.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p release
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > release/operator.yaml

.PHONY: image
image: image-build image-push ## Build and push image with the manager.

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
YQ ?= $(LOCALBIN)/yq
YAMLFMT ?= $(LOCALBIN)/yamlfmt
CRD_REF_DOCS ?= $(LOCALBIN)/crd-ref-docs

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.17.2
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v2.8.0
YQ_VERSION ?= v4.45.3
YAMLFMT_VERSION ?= v0.12.0
CRD_REF_DOCS_VERSION = v0.2.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(LOCALBIN)
	$(call go-install-tool,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))

.PHONY: yamlfmt
yamlfmt: $(YAMLFMT) ## Download yamlfmt locally if necessary.
$(YAMLFMT): $(LOCALBIN)
	$(call go-install-tool,$(YAMLFMT),github.com/google/yamlfmt/cmd/yamlfmt,$(YAMLFMT_VERSION))

.PHONY: crd-ref-docs
crd-ref-docs: $(CRD_REF_DOCS) ## Download crd-ref-docs locally if necessary.
$(CRD_REF_DOCS): $(LOCALBIN)
	$(call go-install-tool,$(CRD_REF_DOCS),github.com/elastic/crd-ref-docs,$(CRD_REF_DOCS_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) CGO_ENABLED=0 go install -v $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

# yq-fmt runs yq with the given expression on a file, then formats it with our preferred YAML style
# $1 - yq expression
# $2 - target file
define yq-fmt
	$(YQ) -i $(1) $(2)
	$(YAMLFMT) -formatter indentless_arrays=true,retain_line_breaks=true $(2)
endef

# json-fmt runs yq with the given expression on a JSON file, then formats it nicely
# $1 - yq expression
# $2 - target JSON file
define json-fmt
	$(YQ) -i $(1) $(2)
	$(YQ) -i -o json -I 2 '.' $(2)
endef

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
ifeq (, $(shell which operator-sdk 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
else
OPERATOR_SDK = $(shell which operator-sdk)
endif
endif

.PHONY: api-docs
API_DOCS_PATH = ./docs/api-overview.md
api-docs: crd-ref-docs ## Creates API docs using https://github.com/elastic/crd-ref-docs
	mkdir -p docs
	$(CRD_REF_DOCS) --source-path ./ --output-path $(API_DOCS_PATH) --renderer markdown --config ./crd-ref-docs.config.yaml
	@# Combined command to remove .io links, ensure a trailing newline, and collapse multiple blank lines.
	@sed -i.bak -e  '/^$$/N;/^\n$$/D' $(API_DOCS_PATH)
	@# BSD sed doesn't generate trailing newlines, so no need to remove them.
	@# BSD sed does not have a '--version' flag, so we need to check for it.
	@if sed --version >/dev/null 2>&1; then \
		sed -i.bak -e '$${/^$$/d}' -e '$${N;/^\n$$/d}' $(API_DOCS_PATH); \
	fi
	rm -f $(API_DOCS_PATH).bak

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS)
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) image-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = $(LOCALBIN)/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.23.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool $(CONTAINER_TOOL) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) image-push IMG=$(CATALOG_IMG)

# Pre-commit checks called explicitly
.PHONY: pre-commit
pre-commit:
	pre-commit run --show-diff-on-failure --color=always --all-files

##@ Release

.PHONY: release
release: yq kustomize yamlfmt ## Prepare release files with VERSION and LLAMASTACK_VERSION
	@if [ "$(LLAMASTACK_VERSION)" = "latest" ]; then \
		echo "Error: LLAMASTACK_VERSION must be explicitly set for releases."; \
		echo "Usage: make release VERSION=0.2.1 LLAMASTACK_VERSION=0.2.12"; \
		exit 1; \
	fi
	@echo "Preparing release with operator version $(VERSION) and LlamaStack version $(LLAMASTACK_VERSION)"

	# Update distributions.json with LlamaStack version and format as pretty JSON
	$(call json-fmt,'to_entries | map(.value |= sub(":latest"; ":$(LLAMASTACK_VERSION)")) | from_entries',distributions.json)

	# Update kustomization files using Kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=quay.io/llamastack/llama-stack-k8s-operator:v$(VERSION)

	# Update environment variables in manager.yaml and format with our preferred YAML style
	# using YQ because Kustomize doesn't support setting environment variables
	$(call yq-fmt,'(select(.kind == "Deployment") | .spec.template.spec.containers[].env[] | select(.name == "OPERATOR_VERSION") | .value) = "$(VERSION)"',config/manager/manager.yaml)

	# Generate manifests and build installer
	$(MAKE) manifests generate
	$(MAKE) -e IMG=quay.io/llamastack/llama-stack-k8s-operator:v$(VERSION) build-installer
