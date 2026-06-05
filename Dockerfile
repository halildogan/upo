# ---------------------------------------------------------------------------
# Build the manager binary using a pinned, reproducible toolchain.
# ---------------------------------------------------------------------------
FROM golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Copy module manifests (go.sum optional) and the Go sources.
COPY go.mod go.sum* ./
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

# Build a static, stripped binary for the target platform. CGO is disabled to
# produce a fully static binary suitable for the distroless static base image.
# GOFLAGS=-mod=mod lets the build resolve the module graph and write go.sum on
# the fly, since go.sum is not committed in this repo. Once go.sum is committed
# you can drop -mod=mod and restore a cached `COPY go.mod go.sum` + download layer.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} GOFLAGS=-mod=mod \
    go build -trimpath -ldflags="-s -w" -a -o manager cmd/main.go

# ---------------------------------------------------------------------------
# Use distroless as minimal base image to package the manager binary.
# Refer to https://github.com/GoogleContainerTools/distroless for more details.
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
