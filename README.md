## To Run
go inside the `test-app` directory, and run:
```bash
docker build -t test-app:v3 .
```
Then, load the image in minikube with:
```bash
minikube image load test-app:v3
```
Now, you can apply the `base_deployment.yaml` and `frontend.yaml` deployment + service files, with:
```bash
kubectl apply -f <filename>
```

Optionally, start the kubernetes dashboard with:
```bash
minikube dashboard
```
### To Run the Operator
Find the instruction in [OPERATOR_DEMO.md](https://github.com/Reckadon/CN_Project_kubernetes_self-healing/blob/main/OPERATOR_DEMO.md)
