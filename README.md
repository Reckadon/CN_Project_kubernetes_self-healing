## To Run
go inside the `test-app` directory, and run:
```bash
docker build -t test-app:v2 .
```
Then, load the image in minikube with:
```bash
minikube image load test-app:v2
```
Now, you can apply the `base_deployment.yaml` and `frontend.yaml` deployment + service files.

Optionally, start the kubernetes dashboard with:
```bash
minikube dashboard
```
