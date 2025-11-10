**Project Description:**

Kubernetes provides built-in self-healing features like restarting failed pods or rescheduling workloads when nodes go down. However, it does not automatically recover from networking-related failures such as CNI plugin crashes, CoreDNS issues, or broken NetworkPolicies.

In this project, a custom Kubernetes operator/controller will be developed to detect and fix such failures. The operator will monitor:

- CNI plugin health (Calico/Flannel pods)
- DNS health (CoreDNS latency and failures)
- Pod-to-pod connectivity (via periodic probes)
- NetworkPolicy enforcement (detect unreachable services)

When a problem is detected, the operator will apply automated remediation  e.g., restarting pods, reapplying NetworkPolicies, or switching to a backup DNS configuration..

Tools:

- Kubernetes (Minikube/KIND)
- Prometheus for health metrics + Alertmanager
- Python (client-go) or Go (Kubebuilder) for operator development
- Calico/Flannel as CNI plugin
- Loki/Grafana for log monitoring (optional)

Expected Learning Outcomes:

- Understand limits of Kubernetes’ built-in self-healing
- Learn how to extend Kubernetes with custom controllers
- Debug and recover networking failures in clusters
- Gain hands-on experience with Kubernetes monitoring and networking stack
