/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	// appsv1 "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sharduljunagade/coredns-operator/api/v1alpha1"
)

// DNSMonitorReconciler reconciles a DNSMonitor object
type DNSMonitorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.sharduljunagade.github.io,resources=dnsmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.sharduljunagade.github.io,resources=dnsmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.sharduljunagade.github.io,resources=dnsmonitors/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods;pods/status;services;namespaces,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;get;list;watch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch


// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DNSMonitor object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile

func (r *DNSMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch DNSMonitor instance
	var dm infrav1alpha1.DNSMonitor
	if err := r.Get(ctx, req.NamespacedName, &dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	ns := dm.Spec.Namespace
	if ns == "" {
		ns = "kube-system"
	}
	interval := dm.Spec.ProbeIntervalSeconds
	if interval == 0 {
		interval = 30
	}
	testDomain := dm.Spec.TestDomain
	if testDomain == "" {
		testDomain = "kubernetes.default.svc.cluster.local"
	}
	failThreshold := dm.Spec.FailureThreshold
	if failThreshold == 0 {
		failThreshold = 3
	}

	// Step 1a: Enforce CoreDNS desired replicas if specified
	if dm.Spec.DesiredReplicas != nil {
		var dep appsv1.Deployment
		if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: "coredns"}, &dep); err == nil {
			// Some clusters label as kube-dns, but deployment name is usually coredns in recent k8s
			current := int32(0)
			if dep.Spec.Replicas != nil {
				current = *dep.Spec.Replicas
			}
			desired := *dm.Spec.DesiredReplicas
			if current < desired {
				dep.Spec.Replicas = &desired
				if err := r.Update(ctx, &dep); err == nil {
					logger.Info("Scaled CoreDNS deployment to desired replicas", "from", current, "to", desired)
					// record status
					dm.Status.LastAction = fmt.Sprintf("Scaled CoreDNS from %d to %d replicas", current, desired)
					_ = r.Status().Update(ctx, &dm)
				}
			}
		}
	}

	// Step 1b: Check CoreDNS pods readiness
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(ns), client.MatchingLabels{"k8s-app": "kube-dns"}); err != nil {
		logger.Error(err, "failed to list CoreDNS pods")
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, err
	}

	unready := 0
	for _, p := range podList.Items {
		if !isPodReady(&p) {
			unready++
		}
	}
	if unready > 0 {
		logger.Info("Detected unready CoreDNS pods", "count", unready)
		for _, p := range podList.Items {
			if !isPodReady(&p) {
				_ = r.Delete(ctx, &p)
				logger.Info("Deleted unready CoreDNS pod", "pod", p.Name)
			}
		}
		dm.Status.LastAction = fmt.Sprintf("Deleted %d unready CoreDNS pods", unready)
		dm.Status.LastChecked = metav1.Now()
		dm.Status.Healthy = false
		_ = r.Status().Update(ctx, &dm)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Step 2: Run a small Job to test DNS resolution
	jobName := fmt.Sprintf("dns-probe-%s", strings.ToLower(req.Name))
	job := buildDNSProbeJob(jobName, ns, testDomain)
	// Set owner ref so GC can clean it up if the CR is deleted
	if err := ctrl.SetControllerReference(&dm, job, r.Scheme); err != nil {
		logger.Error(err, "failed to set owner reference on job")
	}

	// Delete any old job if exists
	_ = r.Delete(ctx, job)

	if err := r.Create(ctx, job); err != nil {
		logger.Error(err, "failed to create probe job")
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, err
	}

	// Wait briefly for job completion (simple synchronous wait)
	time.Sleep(8 * time.Second)

	// Re-fetch job to inspect completion status
	var jobStatus batchv1.Job
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: jobName}, &jobStatus); err != nil {
		logger.Error(err, "could not get probe job status")
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	success := jobStatus.Status.Succeeded > 0
	if !success {
		dm.Status.FailCount++
		logger.Info("DNS probe failed", "failCount", dm.Status.FailCount)
	} else {
		dm.Status.FailCount = 0
		dm.Status.Healthy = true
	}

	// Step 3: If failures exceed threshold → restart CoreDNS pods
	if dm.Status.FailCount >= failThreshold {
		logger.Info("DNS failures exceed threshold — restarting CoreDNS")
		for _, p := range podList.Items {
			_ = r.Delete(ctx, &p)
			logger.Info("Restarted CoreDNS pod", "pod", p.Name)
		}
		dm.Status.LastAction = "Restarted CoreDNS pods due to DNS failures"
		dm.Status.FailCount = 0
		dm.Status.Healthy = false
	}

	// Cleanup probe job
	_ = r.Delete(ctx, job)

	// Update status
	dm.Status.LastChecked = metav1.Now()
	_ = r.Status().Update(ctx, &dm)

	return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
}

// Helper: check pod readiness
func isPodReady(p *corev1.Pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// Helper: build DNS probe Job
func buildDNSProbeJob(name, namespace, testDomain string) *batchv1.Job {
	backoff := int32(0)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "dns-check",
							Image: "busybox:latest",
							Command: []string{"/bin/sh", "-c",
								fmt.Sprintf("nslookup %s || exit 1", testDomain)},
						},
					},
				},
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DNSMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.DNSMonitor{}).
		// Named("dnsmonitor").
		Complete(r)
}
