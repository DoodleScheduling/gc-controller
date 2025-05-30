/*
Copyright 2022 Doodle.

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

package controllers

import (
	"context"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	Keep   int
	MaxAge time.Duration
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

type PodReconcilerOptions struct {
	MaxConcurrentReconciles int
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager, opts PodReconcilerOptions) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: opts.MaxConcurrentReconciles}).
		Complete(r)
}

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := corev1.Pod{}

	err := r.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	r.Log.V(1).Info("reconciling pod", "pod", pod.Name, "namespace", pod.Namespace, "phase", pod.Status.Phase, "reason", pod.Status.Reason, "pod-age", pod.CreationTimestamp, "max-age", r.MaxAge, "keep", r.Keep)

	if pod.Status.Phase != corev1.PodFailed || pod.Status.Reason != "Evicted" {
		return ctrl.Result{}, nil
	}

	if r.MaxAge != 0 && pod.CreationTimestamp.Add(r.MaxAge).Before(time.Now()) {
		r.Log.Info("garbage collect pod due max age", "pod", pod.Name, "namespace", pod.Namespace)
		if err := r.Delete(ctx, &pod); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	var podList corev1.PodList

	if err := r.List(ctx, &podList, client.InNamespace(pod.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	r.Log.V(1).Info("check pod workload children for garbage collection", "pod", pod.Name, "namespace", pod.Namespace)

	gc := podList.Items[:0]
	for _, p := range podList.Items {
		matchedFailedPod := false

		for _, ownerRefManagedPod := range p.GetOwnerReferences() {
			for _, ownerRefEvictedPod := range pod.GetOwnerReferences() {
				if ownerRefEvictedPod.String() == ownerRefManagedPod.String() && p.Status.Phase == corev1.PodFailed && p.Status.Reason == "Evicted" {
					matchedFailedPod = true
					break
				}
			}

		}

		if matchedFailedPod {
			gc = append(gc, p)
		}
	}

	sort.Slice(gc, func(i, j int) bool {
		return gc[i].GetObjectMeta().GetCreationTimestamp().After(gc[j].GetCreationTimestamp().Time)
	})

	keep := r.Keep
	if len(gc) < r.Keep {
		keep = len(gc)
	}

	for _, gcPod := range gc[keep:] {
		r.Log.Info("delete evicted pod", "pod", gcPod.Name, "namespace", gcPod.Namespace)

		if err := r.Delete(ctx, &gcPod); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{
		RequeueAfter: r.MaxAge,
	}, nil
}
