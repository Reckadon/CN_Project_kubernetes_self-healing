# Kubernetes Self-Healing Application Demo

This project demonstrates Kubernetes' self-healing capabilities using a Flask application that randomly fails and a monitoring system that tracks the recovery process.

## 🏗️ Architecture Overview

### Components

1. **Flask Application (`test_app/`)**
   - A simple web app that responds with "Hello, I'm alive!"
   - **Failure Simulation**: 20% chance to timeout (sleep 60s) on each request
   - Built into a Docker image (`test-app:latest`)

2. **Base Deployment (`base_deployment.yaml`)**
   - **Deployment**: Runs 3 replicas of the Flask app for high availability
   - **Service**: Exposes the app internally via `baseline-svc` service
   - **Self-Healing**: Liveness probes monitor container health and restart failed instances

3. **Frontend Monitor (`frontend.yaml`)**
   - Continuous monitoring pod using curl
   - Tests the application every 2 seconds
   - Logs success/failure with timestamps to demonstrate healing

### How Self-Healing Works

1. **Detection**: Kubernetes liveness probes check `/` endpoint every 20 seconds
2. **Failure**: When the app times out, the probe fails after 2 consecutive failures
3. **Recovery**: Kubernetes automatically restarts the failing container
4. **Load Balancing**: Traffic is distributed across healthy replicas during recovery

## 🚀 Quick Start Guide

### Prerequisites

- Docker installed and running
- Minikube installed and running
- kubectl configured for your cluster

### Step 1: Build the Application

```bash
# Navigate to the project directory
cd /path/to/CN_Project_kubernetes_self-healing

# Build the Docker image
cd test_app
docker build -t test-app:latest .
cd ..

# Load image into Minikube
minikube image load test-app:latest
```

### Step 2: Deploy the Application

```bash
# Deploy the main application and service
kubectl apply -f base_deployment.yaml

# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l app=baseline-app --timeout=60s
```

### Step 3: Deploy the Monitor

```bash
# Deploy the monitoring frontend
kubectl apply -f frontend.yaml

# Wait for monitor pod to be ready
kubectl wait --for=condition=ready pod -l app=frontend --timeout=60s
```

### Step 4: Watch the Self-Healing in Action

```bash
# Watch the monitor logs to see success/failure patterns
kubectl logs -f deployment/frontend

# In another terminal, watch pod restarts
kubectl get pods -l app=baseline-app -w
```

## 🔍 Monitoring and Testing

### View Application Logs
```bash
# Check main application logs
kubectl logs -l app=baseline-app

# Check monitor logs
kubectl logs -l app=frontend
```

### Check Pod Health Status
```bash
# View all pods
kubectl get pods

# Watch pods for restart events
kubectl get pods -l app=baseline-app -w

# Describe a specific pod for detailed events
kubectl describe pod <pod-name>
```

### Manual Testing
```bash
# Port forward to test manually (in separate terminal)
kubectl port-forward service/baseline-svc 8080:80

# Test with curl (in another terminal)
curl http://localhost:8080
```

## 📊 Expected Behavior

### Normal Operation
You should see logs like:
```
2025-11-09T10:00:00Z OK
2025-11-09T10:00:02Z OK
2025-11-09T10:00:04Z FAIL    # App timed out
2025-11-09T10:00:06Z FAIL    # Still failing
2025-11-09T10:00:08Z OK      # Kubernetes restarted the container
```

### Pod Restart Events
Watch for:
- **RESTART** count increases when containers are restarted
- Brief periods where pod count might be less than 3
- Automatic recovery without manual intervention

## 🛠️ Configuration Details

### Liveness Probe Settings
- **Path**: `/` (root endpoint)
- **Initial Delay**: 10 seconds (wait for app to start)
- **Check Interval**: 20 seconds
- **Failure Threshold**: 2 consecutive failures trigger restart

### Application Settings
- **Failure Rate**: 20% of requests timeout
- **Timeout Duration**: 60 seconds (simulates hung process)
- **Replicas**: 3 for high availability

## 🧹 Cleanup

```bash
# Remove all deployed resources
kubectl delete -f frontend.yaml
kubectl delete -f base_deployment.yaml

# Stop Minikube (optional)
minikube stop
```

## 🐛 Troubleshooting

### Image Pull Issues
```bash
# Verify image is loaded in Minikube
minikube image ls | grep test-app

# Reload if necessary
minikube image load test-app:latest
```

### Pods Not Starting
```bash
# Check pod events
kubectl describe pods -l app=baseline-app

# Check if Minikube is running
minikube status
```

### No Self-Healing Observed
- Wait longer - probes take time to fail (40+ seconds)
- Check probe configuration in the deployment
- Verify the app is actually failing (check logs)

## 📚 Learning Points

1. **Kubernetes Resilience**: How K8s automatically handles container failures
2. **Health Checks**: Importance of proper liveness and readiness probes
3. **Load Balancing**: How traffic is distributed across healthy replicas
4. **Monitoring**: Observing system behavior through logs and events
5. **High Availability**: Using multiple replicas to maintain service during failures

This demo showcases why Kubernetes is essential for production workloads - it provides automatic recovery from common failure scenarios without human intervention.