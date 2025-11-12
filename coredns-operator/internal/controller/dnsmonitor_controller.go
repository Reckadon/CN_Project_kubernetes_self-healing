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

	// 1. Enforce CoreDNS desired replicas if specified
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

	// 2. Check CoreDNS pods readiness
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
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	// ---------- PROBE JOB HANDLING (replacement) ----------
	jobName := fmt.Sprintf("dns-probe-%s", strings.ToLower(req.Name))
	var existingJob batchv1.Job
	jobKey := client.ObjectKey{Namespace: ns, Name: jobName}
	jobFound := true
	if err := r.Get(ctx, jobKey, &existingJob); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "error fetching probe job")
			return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
		}
		jobFound = false
	}

	if jobFound {
		// If job is still active (no succeeded/failed pods), skip creating a new job.
		if existingJob.Status.Active > 0 {
			logger.Info("Probe job already running; skipping creation", "job", jobName)
			// Update status.LastChecked and requeue after interval
			dm.Status.LastChecked = metav1.Now()
			_ = r.Status().Update(ctx, &dm)
			return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
		}

		// If job completed (succeeded or failed), evaluate result
		if existingJob.Status.Succeeded > 0 {
			// probe success
			dm.Status.FailCount = 0
			dm.Status.Healthy = true
			logger.Info("Probe job succeeded", "job", jobName)
		} else {
			// probe failed
			dm.Status.FailCount++
			dm.Status.Healthy = false
			logger.Info("Probe job failed or did not succeed", "failCount", dm.Status.FailCount)
		}

		// Clean up completed job (best-effort)
		_ = r.Delete(ctx, &existingJob)
		// persist status
		dm.Status.LastChecked = metav1.Now()
		_ = r.Status().Update(ctx, &dm)

		// If failures exceed threshold, remediation below (shared with other logic)
		if dm.Status.FailCount >= failThreshold {
			logger.Info("DNS failures >= threshold -> performing remediation")
			// remediation block continues below (delete pods or rolling restart)
		} else {
			// No remediation needed; requeue after interval
			return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
		}
	}

	// If no existing job, create one
	if !jobFound {
		job := buildDNSProbeJob(jobName, ns, testDomain)
		if err := ctrl.SetControllerReference(&dm, job, r.Scheme); err != nil {
			logger.Error(err, "failed to set owner reference on job")
		}
		if err := r.Create(ctx, job); err != nil {
			logger.Error(err, "failed to create probe job")
			// try again later
			return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
		}
		logger.Info("Created probe job", "job", jobName)
		// Return and wait — job completion will requeue controller if SetupWithManager Owns(Job) is set
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	// ---------- REMEDIATION (delete pods) ----------
	if dm.Status.FailCount >= failThreshold {
		logger.Info("Restarting CoreDNS pods due to consecutive DNS probe failures")
		for _, p := range podList.Items {
			if err := r.Delete(ctx, &p); err != nil {
				logger.Error(err, "failed to delete CoreDNS pod", "pod", p.Name)
			} else {
				logger.Info("Deleted CoreDNS pod", "pod", p.Name)
			}
		}
		dm.Status.LastAction = "Restarted CoreDNS pods due to DNS failures"
		dm.Status.FailCount = 0
		dm.Status.Healthy = false
		dm.Status.LastChecked = metav1.Now()
		_ = r.Status().Update(ctx, &dm)
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	// default requeue
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
	Owns(&batchv1.Job{}).
	// Named("dnsmonitor").
	Complete(r)
}
