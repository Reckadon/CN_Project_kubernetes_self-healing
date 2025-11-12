```

mkdir -p coredns-operator && cd coredns-operator

kubebuilder init --domain sharduljunagade.github.io --repo github.com/sharduljunagade/coredns-operator

kubebuilder create api --group infra --version v1alpha1 --kind DNSMonitor

# Edit api/v1alpha1/dnsmonitor_types.go to define the spec and status

```

docker build -t test-app:v3 .
minikube image load test-app:v3
kubectl apply -f ../base_deployment.yaml

```

# make sure to run
make generate
make manifests


# edit controllers/dnsmonitor_controller.go to implement the reconciliation logic

# Build and push the operator image
IMG=shardul0109/coredns-operator:v1
make docker-build docker-push IMG=${IMG}

# Deploy the operator
minikube image load ${IMG}
make deploy IMG=${IMG}


# add to roles.yaml
- apiGroups: [""]
  resources: ["pods", "pods/status", "services"]
  verbs: ["get","list","watch","delete"]
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get","list","watch","update","patch"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create","get","list","watch","delete"]


make manifests
make install

# create a sample CR - configs/samples/infra_v1alpha1_dnsmonitor.yaml
apiVersion: infra.your.domain/v1alpha1
kind: DNSMonitor
metadata:
  name: dns-monitor
  namespace: default
spec:
  namespace: kube-system
  probeIntervalSeconds: 30
  testDomain: "kubernetes.default.svc.cluster.local"
  failureThreshold: 3


# Create a sample DNSMonitor resource
make install
kubectl apply -f config/samples/infra_v1alpha1_dnsmonitor.yaml


# Verify the CRD is created
kubectl get crds | grep dns


make deploy IMG=${IMG}

kubectl get pods -n coredns-operator-system
kubectl get dnsmonitors -A
kubectl describe dnsmonitor dns-monitor


kubectl delete pods -n kube-system -l k8s-app=kube-dns
kubectl delete pods --all -n kube-system
```





kubectl delete -f config/samples/infra_v1alpha1_dnsmonitor.yaml
make undeploy
make uninstall
kubectl delete -f ../frontend.yaml || true
kubectl delete -f ../base_deployment.yaml || true