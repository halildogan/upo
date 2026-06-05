/*
Copyright 2026 The Unified Platform Operator Authors.

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

package platform

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// ConnectionSecretSuffix is appended to an Integration's name to form the
// operator-managed connection Secret that workloads consume.
const ConnectionSecretSuffix = "-connection"

// ConnectionSecretName returns the deterministic connection Secret name.
func ConnectionSecretName(integrationName string) string {
	return integrationName + ConnectionSecretSuffix
}

// ProbeRequest is a transport-agnostic health probe description.
type ProbeRequest struct {
	Endpoint           string
	HealthPath         string
	Timeout            time.Duration
	InsecureSkipVerify bool
	Headers            map[string]string
}

// Prober performs an active connectivity check against an external endpoint.
// It is an interface so the controller can be unit-tested with a fake prober
// and so non-HTTP connector families can supply their own implementations.
type Prober interface {
	Probe(ctx context.Context, req ProbeRequest) error
}

// HTTPProber probes HTTP(S) endpoints. It honors HTTPS_PROXY/HTTP_PROXY from
// the environment (required in egress-restricted clusters) and enforces a
// modern TLS floor unless explicitly told to skip verification.
type HTTPProber struct{}

// Probe issues a GET to endpoint+healthPath and treats any non-5xx, non-error
// response as healthy (4xx often means "auth required", i.e. reachable).
func (HTTPProber) Probe(ctx context.Context, req ProbeRequest) error {
	target := strings.TrimRight(req.Endpoint, "/")
	path := req.HealthPath
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target += path

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: req.InsecureSkipVerify, //nolint:gosec // operator-controlled, defaults to false
		},
	}
	httpClient := &http.Client{Timeout: req.Timeout, Transport: transport}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("probe %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("probe %s returned %d", target, resp.StatusCode)
	}
	return nil
}

// IsHTTPType reports whether the connector family is reachable over HTTP and so
// can be health-probed by the HTTPProber.
func IsHTTPType(t platformv1alpha1.IntegrationType) bool {
	switch t {
	case platformv1alpha1.IntegrationTypeWebhook,
		platformv1alpha1.IntegrationTypeRESTAPI,
		platformv1alpha1.IntegrationTypeOAuth2,
		platformv1alpha1.IntegrationTypeObjectStore:
		return true
	default:
		return false
	}
}

// ResolveAuthSecret loads the credentials Secret referenced by an Integration.
// Returns (nil, nil) when no AuthSecretRef is set. A NotFound is surfaced so the
// controller can report a DependencyNotMet condition and requeue.
func ResolveAuthSecret(ctx context.Context, c client.Client, ns string, ref *platformv1alpha1.SecretReference) (map[string][]byte, error) {
	if ref == nil {
		return nil, nil
	}
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: ns, Name: ref.Name}
	if err := c.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	return secret.Data, nil
}

// BuildConnectionData assembles the normalized, workload-facing connection
// payload: the validated endpoint and type, plus any resolved credential keys.
func BuildConnectionData(spec platformv1alpha1.IntegrationSpec, creds map[string][]byte) map[string][]byte {
	data := map[string][]byte{
		"endpoint": []byte(spec.Endpoint),
		"type":     []byte(spec.Type),
	}
	for k, v := range creds {
		// Namespacing credential keys avoids collisions with the reserved keys.
		data["credential."+k] = v
	}
	return data
}

// ReconcileConnectionSecret converges the operator-managed connection Secret in
// the Integration's own namespace, owned by the Integration so it is garbage
// collected on deletion.
func ReconcileConnectionSecret(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	namespace, name string,
	data map[string][]byte,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	return controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		secret.Labels = MergeLabels(secret.Labels, labels)
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = data
		return controllerutil.SetControllerReference(owner, secret, scheme)
	})
}

// DeleteConnectionSecret removes the managed connection Secret (finalizer path).
func DeleteConnectionSecret(ctx context.Context, c client.Client, namespace, name string) error {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if err := c.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
