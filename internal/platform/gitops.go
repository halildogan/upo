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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// GVKs for the GitOps engines the operator can delegate to. They are referenced
// as unstructured objects so the operator does not take a hard compile-time
// dependency on Argo CD or Flux APIs; the CRDs simply need to be installed in
// the cluster when the corresponding provider is selected.
var (
	argoApplicationGVK = schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"}
	fluxGitRepoGVK     = schema.GroupVersionKind{Group: "source.toolkit.fluxcd.io", Version: "v1", Kind: "GitRepository"}
	fluxKustomizeGVK   = schema.GroupVersionKind{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Kind: "Kustomization"}
)

// GitOpsResourceName derives a deterministic, DNS-safe child resource name.
func GitOpsResourceName(envName string) string { return "upo-" + envName }

// ReconcileGitOps converges the GitOps delivery objects for an environment and
// returns the observed synchronization status. For provider "none" it is a
// no-op returning (nil, nil) so callers can mark the environment Healthy on the
// strength of namespace provisioning alone.
func ReconcileGitOps(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	env *platformv1alpha1.Environment,
	targetNamespace string,
) (*platformv1alpha1.SyncStatus, error) {
	switch env.Spec.GitOps.Provider {
	case platformv1alpha1.GitOpsProviderArgoCD:
		return reconcileArgo(ctx, c, scheme, env, targetNamespace)
	case platformv1alpha1.GitOpsProviderFlux:
		return reconcileFlux(ctx, c, scheme, env, targetNamespace)
	default:
		return nil, nil
	}
}

// DeleteGitOps removes any GitOps child objects this operator created for the
// environment. Owner references would garbage-collect them anyway, but explicit
// deletion in the finalizer guarantees the external engine stops syncing before
// the workload namespace is torn down.
func DeleteGitOps(ctx context.Context, c client.Client, env *platformv1alpha1.Environment) error {
	name := GitOpsResourceName(env.Name)
	gvks := []schema.GroupVersionKind{argoApplicationGVK, fluxKustomizeGVK, fluxGitRepoGVK}
	for _, gvk := range gvks {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)
		obj.SetName(name)
		obj.SetNamespace(env.Namespace)
		if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) && !isNoMatch(err) {
			return err
		}
	}
	return nil
}

func reconcileArgo(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	env *platformv1alpha1.Environment,
	targetNamespace string,
) (*platformv1alpha1.SyncStatus, error) {
	if env.Spec.Source == nil {
		return nil, fmt.Errorf("spec.source is required when gitops.provider=argocd")
	}
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(argoApplicationGVK)
	app.SetName(GitOpsResourceName(env.Name))
	app.SetNamespace(env.Namespace)

	if _, err := controllerutil.CreateOrUpdate(ctx, c, app, func() error {
		app.SetLabels(MergeLabels(app.GetLabels(), gitopsLabels(env)))
		spec := map[string]interface{}{
			"project": "default",
			"source": map[string]interface{}{
				"repoURL":        env.Spec.Source.RepoURL,
				"targetRevision": env.Spec.Source.Revision,
				"path":           env.Spec.Source.Path,
			},
			"destination": map[string]interface{}{
				"server":    "https://kubernetes.default.svc",
				"namespace": targetNamespace,
			},
		}
		if env.Spec.GitOps.AutoSync {
			spec["syncPolicy"] = map[string]interface{}{
				"automated": map[string]interface{}{
					"prune":    env.Spec.GitOps.Prune,
					"selfHeal": true,
				},
			}
		}
		if err := unstructured.SetNestedMap(app.Object, spec, "spec"); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(env, app, scheme)
	}); err != nil {
		return nil, err
	}

	syncPhase, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
	revision, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")
	health, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
	return &platformv1alpha1.SyncStatus{
		Phase:    normalizeArgoSync(syncPhase),
		Revision: revision,
		Message:  fmt.Sprintf("argocd health=%s sync=%s", emptyToUnknown(health), emptyToUnknown(syncPhase)),
	}, nil
}

func reconcileFlux(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	env *platformv1alpha1.Environment,
	targetNamespace string,
) (*platformv1alpha1.SyncStatus, error) {
	if env.Spec.Source == nil {
		return nil, fmt.Errorf("spec.source is required when gitops.provider=flux")
	}
	name := GitOpsResourceName(env.Name)

	repo := &unstructured.Unstructured{}
	repo.SetGroupVersionKind(fluxGitRepoGVK)
	repo.SetName(name)
	repo.SetNamespace(env.Namespace)
	if _, err := controllerutil.CreateOrUpdate(ctx, c, repo, func() error {
		repo.SetLabels(MergeLabels(repo.GetLabels(), gitopsLabels(env)))
		spec := map[string]interface{}{
			"interval": "1m0s",
			"url":      env.Spec.Source.RepoURL,
			"ref":      map[string]interface{}{"name": env.Spec.Source.Revision},
		}
		if err := unstructured.SetNestedMap(repo.Object, spec, "spec"); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(env, repo, scheme)
	}); err != nil {
		return nil, err
	}

	ks := &unstructured.Unstructured{}
	ks.SetGroupVersionKind(fluxKustomizeGVK)
	ks.SetName(name)
	ks.SetNamespace(env.Namespace)
	if _, err := controllerutil.CreateOrUpdate(ctx, c, ks, func() error {
		ks.SetLabels(MergeLabels(ks.GetLabels(), gitopsLabels(env)))
		spec := map[string]interface{}{
			"interval":        "1m0s",
			"path":            env.Spec.Source.Path,
			"prune":           env.Spec.GitOps.Prune,
			"targetNamespace": targetNamespace,
			"sourceRef": map[string]interface{}{
				"kind": "GitRepository",
				"name": name,
			},
		}
		if err := unstructured.SetNestedMap(ks.Object, spec, "spec"); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(env, ks, scheme)
	}); err != nil {
		return nil, err
	}

	revision, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")
	ready := readyConditionStatus(ks.Object)
	return &platformv1alpha1.SyncStatus{
		Phase:    normalizeFluxReady(ready),
		Revision: revision,
		Message:  fmt.Sprintf("flux kustomization ready=%s", emptyToUnknown(ready)),
	}, nil
}

func gitopsLabels(env *platformv1alpha1.Environment) map[string]string {
	l := BaseLabels()
	l[TenantLabel] = env.Spec.TenantRef.Name
	l[EnvironmentLabel] = env.Name
	l[ComponentLabel] = "gitops"
	return l
}

// readyConditionStatus extracts the status of the "Ready" condition from a
// Flux object's status.conditions array.
func readyConditionStatus(obj map[string]interface{}) string {
	conds, found, err := unstructured.NestedSlice(obj, "status", "conditions")
	if err != nil || !found {
		return ""
	}
	for _, raw := range conds {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := cond["type"].(string); t == "Ready" {
			s, _ := cond["status"].(string)
			return s
		}
	}
	return ""
}

func normalizeArgoSync(s string) string {
	switch s {
	case "Synced":
		return "Synced"
	case "OutOfSync":
		return "OutOfSync"
	case "":
		return "Unknown"
	default:
		return "Error"
	}
}

func normalizeFluxReady(s string) string {
	switch s {
	case "True":
		return "Synced"
	case "False":
		return "Error"
	default:
		return "Unknown"
	}
}

func emptyToUnknown(s string) string {
	if s == "" {
		return "Unknown"
	}
	return s
}

// isNoMatch reports whether err indicates the GitOps CRD is not installed in
// the cluster (no REST mapping). Treated as benign during deletion so teardown
// never blocks on an absent optional engine.
func isNoMatch(err error) bool {
	return meta.IsNoMatchError(err)
}
