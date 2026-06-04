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

// Command manager is the entrypoint for the Unified Platform Operator control
// plane. It wires the Tenant, Environment, and Integration controllers and
// their admission webhooks into a single controller-runtime manager with secure
// metrics, leader election, health probes, and OpenTelemetry tracing.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC) to ensure
	// out-of-cluster (kubeconfig) execution works against managed clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
	"github.com/halildogan/upo/internal/controller"
	"github.com/halildogan/upo/internal/telemetry"
	webhookv1alpha1 "github.com/halildogan/upo/internal/webhook/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(platformv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		metricsSecure        bool
		enableLeaderElection bool
		probeAddr            string
		enableHTTP2          bool
		webhookCertPath      string
		webhookCertName      string
		webhookCertKey       string
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. Use :8443 for HTTPS or 0 to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager to ensure a single active manager.")
	flag.BoolVar(&metricsSecure, "metrics-secure", true, "Serve metrics over HTTPS with authn/authz. Set false to serve insecurely.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "Enable HTTP/2 for the metrics and webhook servers (disabled by default to mitigate HTTP/2 vulnerabilities).")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "Directory that contains the webhook serving certificate and key.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	ctx := ctrl.SetupSignalHandler()

	// OpenTelemetry tracing (no-op until an exporter is configured; see docs).
	shutdownTracing, err := telemetry.Setup(ctx, version)
	if err != nil {
		setupLog.Error(err, "unable to set up tracing")
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	// HTTP/2 is disabled unless explicitly enabled to avoid Rapid Reset / stream
	// multiplexing DoS classes (CVE-2023-44487, CVE-2023-39325).
	var tlsOpts []func(*tls.Config)
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) { c.NextProtos = []string{"http/1.1"} })
	}

	webhookTLSOpts := tlsOpts
	webhookServerOpts := webhook.Options{TLSOpts: webhookTLSOpts}
	if webhookCertPath != "" {
		setupLog.Info("serving webhooks with provided certificate", "path", webhookCertPath)
		webhookServerOpts.CertDir = webhookCertPath
		webhookServerOpts.CertName = webhookCertName
		webhookServerOpts.KeyName = webhookCertKey
	}
	webhookServer := webhook.NewServer(webhookServerOpts)

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: metricsSecure,
		TLSOpts:       tlsOpts,
	}
	if metricsSecure {
		// Require authenticated + authorized scrapes (TokenReview/SubjectAccessReview).
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "upo-controller.platform.upo.io",
		// Releasing leadership on graceful shutdown shortens failover time at the
		// small risk of two managers briefly overlapping during rollout.
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.TenantReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tenant-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tenant")
		os.Exit(1)
	}
	if err := (&controller.EnvironmentReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("environment-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Environment")
		os.Exit(1)
	}
	if err := (&controller.IntegrationReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("integration-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Integration")
		os.Exit(1)
	}

	// Admission webhooks. Disable in local `make run` via ENABLE_WEBHOOKS=false.
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha1.SetupTenantWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Tenant")
			os.Exit(1)
		}
		if err := webhookv1alpha1.SetupEnvironmentWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Environment")
			os.Exit(1)
		}
		if err := webhookv1alpha1.SetupIntegrationWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Integration")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "version", version)
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
