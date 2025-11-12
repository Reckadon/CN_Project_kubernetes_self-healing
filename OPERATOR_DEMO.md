# CoreDNS Self-Healing Operator Demo

This guide shows how to reproduce DNS failure in Kubernetes and use the custom operator in this repo to detect and heal it automatically.

## Prerequisites

- Docker
- Minikube
- kubectl
- Go toolchain (for building locally) and make

Repo layout:
- `test_app/` — Flask app used in a separate self-healing demo
- `base_deployment.yaml` / `frontend.yaml` — app and curl monitor
aiming to show liveness-probe healing
- `coredns-operator/` — Kubebuilder-based operator that monitors DNS
and restarts/scales CoreDNS on failures

## Start Minikube

```bash
minikube start
# Optional: enable metrics-server for nicer dashboard metrics
minikube addons enable metrics-server
```

## (Optional) App self-healing demo
These are independent from the DNS operator and useful to verify your cluster is working.
```bash
# Build and load image
cd test_app
docker build -t test-app:v3 .
minikube image load test-app:v3
cd ..

# Deploy app + service + monitor
kubectl apply -f base_deployment.yaml
kubectl apply -f frontend.yaml

# Watch monitor
kubectl logs -f deployment/frontend
```

## Build the DNS Operator (only do once)

```bash
cd coredns-operator

kubebuilder init --domain sharduljunagade.github.io --repo github.com/sharduljunagade/coredns-operator

kubebuilder create api --group infra --version v1alpha1 --kind DNSMonitor

```

## Deploy the DNS Operator

Inside `coredns-operator/`:
```bash
cd coredns-operator

# Generate code and manifests (required after API changes)
make generate
make manifests

# Build/push operator image
export IMG="shardul0109/coredns-operator:v1"
# make docker-build docker-push IMG=${IMG}
make docker-build IMG=${IMG}

# Load to minikube cache (optional if using external registry pull)
minikube image load ${IMG}

# Install CRDs and deploy operator
make install
make deploy IMG=${IMG}

# Verify controller is running
kubectl get pods -n coredns-operator-system
```

## Create the DNSMonitor resource

The sample CR is at `coredns-operator/config/samples/infra_v1alpha1_dnsmonitor.yaml`:
- namespace: `kube-system`
- probeIntervalSeconds: `30`
- testDomain: `kubernetes.default.svc.cluster.local`
- failureThreshold: `3`
- desiredReplicas: `2` (operator enforces minimum number of CoreDNS replicas)

Apply it:
```bash
kubectl apply -f config/samples/infra_v1alpha1_dnsmonitor.yaml -n default
kubectl get dnsmonitors -A
```

Check that the operator is running and monitoring DNS:
```bash
kubectl logs -n coredns-operator-system deploy/coredns-operator-controller-manager -f
```

Inspect CR status:
```bash
kubectl describe dnsmonitor dns-monitor
```

List CoreDNS pods:
```bash
kubectl get pods -n kube-system -l k8s-app=kube-dns -w
```



## Simulate DNS Failure

1) Scale CoreDNS to zero:
```bash
kubectl scale deployment coredns -n kube-system --replicas=0
kubectl get deploy coredns -n kube-system -w
```

2) Verify DNS inside any pod fails:
```bash
kubectl run dnsutils --image=busybox:1.36 -- sleep 3600
kubectl exec -it dnsutils -- nslookup example.com
```

## How the Operator Heals

- On each reconcile (every `probeIntervalSeconds` seconds):
  - Lists CoreDNS pods in `kube-system` with label `k8s-app=kube-dns` and removes any unready ones
  - Ensures the `coredns` deployment has at least `desiredReplicas`
  - Creates a short Job that checks DNS with `nslookup <testDomain>`
  - Increments an internal failure counter in the CR status when probes fail
  - When failures reach `failureThreshold`, deletes CoreDNS pods to force restart

You should quickly see the operator:
- Scale `coredns` back up to `desiredReplicas` when you scale it down to 0
- Delete unready CoreDNS pods when DNS probes fail repeatedly


Re-test DNS:
```bash
kubectl run -it --rm dnsutils --image=busybox:1.36 --restart=Never -- nslookup example.com
```

## Cleanup
When you’re done testing, clean up everything:
```
kubectl delete -f config/samples/infra_v1alpha1_dnsmonitor.yaml
make undeploy
make uninstall

# Optional: cleanup test apps
kubectl delete -f ../frontend.yaml || true
kubectl delete -f ../base_deployment.yaml || true
```


You can also remove leftover successful Jobs:
```
kubectl delete pod -n kube-system --field-selector=status.phase=Succeeded
```

## Notes
- RBAC for pods, services, deployments, and jobs is preconfigured in `config/rbac/role.yaml`.
- The controller uses label selector `k8s-app=kube-dns` to find CoreDNS pods.
- The Deployment is assumed to be named `coredns` in `kube-system` (default for modern clusters).
- Probe job uses `busybox:latest` to run `nslookup`.