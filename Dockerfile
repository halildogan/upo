# ---------------------------------------------------------------------------
# Build the manager binary using a pinned, reproducible toolchain.
# ---------------------------------------------------------------------------
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Copy the Go module manifests first to leverage Docker layer caching: the
# expensive `go mod download` step is only re-run when go.mod / go.sum change.
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the Go sources.
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

# Build a static, stripped binary for the target platform. CGO is disabled to
# produce a fully static binary suitable for the distroless static base image.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
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
