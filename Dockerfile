# Build the manager binary
ARG GOLANG_VERSION=1.25

# Use BUILDPLATFORM to run the builder natively (avoid QEMU emulation for Go compilation)
# This dramatically improves build reliability and speed for cross-platform builds
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/go-toolset:${GOLANG_VERSION} as builder
ARG TARGETOS=linux
ARG TARGETARCH
ARG BUILDPLATFORM
ARG TARGETPLATFORM

# FIPS compliance settings
# For native builds: CGO_ENABLED=1 with full FIPS OpenSSL support
# For cross-builds: CGO_ENABLED=0 with pure Go FIPS (strictfipsruntime)
ENV GOEXPERIMENT=strictfipsruntime
ENV GOTOOLCHAIN=auto

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/
COPY distributions.json distributions.json

# Build the manager binary
# Cross-compilation is handled natively by Go via GOOS and GOARCH
# This runs on the build host's native architecture, not under QEMU emulation
USER root

# Determine if we're cross-compiling by comparing BUILDPLATFORM and TARGETPLATFORM
# - Native builds (same platform): CGO_ENABLED=1 with openssl tag for full FIPS OpenSSL support
# - Cross builds (different platform): CGO_ENABLED=0 with pure Go FIPS (no CGO = no cross-compiler needed)
RUN echo "Building for TARGETPLATFORM=${TARGETPLATFORM} on BUILDPLATFORM=${BUILDPLATFORM}" && \
    if [ "${BUILDPLATFORM}" = "${TARGETPLATFORM}" ]; then \
        echo "Native build detected - using CGO_ENABLED=1 with OpenSSL FIPS"; \
        CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -a -tags=strictfipsruntime,openssl -o manager main.go; \
    else \
        echo "Cross-compilation detected - using CGO_ENABLED=0 with pure Go FIPS"; \
        CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -a -tags=strictfipsruntime -o manager main.go; \
    fi

# Use UBI minimal as the runtime base image
# This stage runs under QEMU for cross-platform builds, but the workload is minimal
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
ARG TARGETARCH

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/controllers/manifests ./manifests/

# Install openssl - use minimal options for reliability under QEMU emulation
RUN microdnf install -y --setopt=install_weak_deps=0 --setopt=tsflags=nodocs openssl && \
    microdnf clean all

USER 1001

ENTRYPOINT ["/manager"]
