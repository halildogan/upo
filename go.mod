module github.com/halildogan/upo

go 1.24.0

// NOTE: This go.mod pins direct dependencies. The transitive graph and go.sum
// are produced by `go mod tidy` / `go mod download` against the module proxy.
// They are intentionally not committed in this review-grade artifact because the
// build sandbox has no network access to proxy.golang.org. Run `make tidy`
// (or `go mod tidy`) once in a networked environment to materialize go.sum.

require (
	github.com/onsi/ginkgo/v2 v2.22.0
	github.com/onsi/gomega v1.36.1
	github.com/prometheus/client_golang v1.20.5
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/sdk v1.34.0
	go.opentelemetry.io/otel/trace v1.34.0
	k8s.io/api v0.32.3
	k8s.io/apimachinery v0.32.3
	k8s.io/client-go v0.32.3
	sigs.k8s.io/controller-runtime v0.20.4
)
