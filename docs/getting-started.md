# Getting Started

This guide takes you from an empty machine to a running operator managing a
tenant, on a local [kind](https://kind.sigs.k8s.io/) cluster.

## Prerequisites

- Go 1.24+
- Docker (or another container runtime)
- `kubectl` 1.29+
- `kind` 0.24+ (for the local cluster)
- `cert-manager` (installed below; required for the admission webhooks)

## 1. Create a cluster

```bash
kind create cluster --name upo-dev
```

## 2. Install cert-manager

The webhooks are served with a cert-manager-issued certificate.

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=120s
```

## 3. Build and load the image

```bash
make docker-build IMG=upo:dev
kind load docker-image upo:dev --name upo-dev
```

## 4. Install CRDs and deploy

Using raw kustomize:

```bash
make install                 # CRDs only
make deploy IMG=upo:dev      # CRDs + RBAC + manager + webhooks + cert
kubectl -n upo-system rollout status deploy/upo-controller-manager
```

…or using Helm:

```bash
helm install upo charts/unified-platform-operator \
  --namespace upo-system --create-namespace \
  --set image.repository=upo --set image.tag=dev
```

## 5. Create your first tenant

```bash
kubectl apply -f examples/tenant.yaml

kubectl get tenants
# NAME   TIER         NAMESPACE     PHASE    READY   AGE
# acme   enterprise   tenant-acme   Active   True    8s

kubectl get ns tenant-acme
kubectl -n tenant-acme get resourcequota,limitrange,networkpolicy,rolebinding
```

## 6. Add an environment and an integration

```bash
kubectl apply -f examples/environment.yaml
kubectl apply -f examples/integration.yaml

kubectl -n tenant-acme get environments,integrations
kubectl -n tenant-acme describe environment staging
```

> The Argo CD / Flux delivery in `examples/environment.yaml` requires the
> respective engine installed. Without it, the Environment still provisions its
> workload namespace and config; the GitOps `Application`/`Kustomization` is
> created once the engine's CRDs are present.

## 7. Observe status & metrics

```bash
kubectl get tenant acme -o jsonpath='{.status.conditions}' | jq
kubectl wait --for=condition=Ready tenant/acme --timeout=60s

# Secure metrics (from inside the cluster, with a bound ServiceAccount token):
kubectl -n upo-system port-forward svc/upo-controller-manager-metrics-service 8443:8443
```

## 8. Run locally (without deploying)

For a fast dev loop, run the manager on your host against the cluster (webhooks
disabled, since they need in-cluster serving certs):

```bash
make install
ENABLE_WEBHOOKS=false make run
```

## 9. Tests

```bash
make test        # unit + envtest integration tests
make test-e2e    # end-to-end against the current kube context
```

## Teardown

```bash
kubectl delete -f examples/ --ignore-not-found
make undeploy
kind delete cluster --name upo-dev
```
