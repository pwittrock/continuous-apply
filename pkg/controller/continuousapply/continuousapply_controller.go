/*
Copyright 2018 The Kubernetes authors.

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

package continuousapply

import (
	"context"
	"fmt"

	applyv1beta1 "github.com/pwittrock/continuous-apply/pkg/apis/apply/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const caImage = "pwittrock/continuous-apply:v24"

// Add creates a new ContinuousApply Controller and adds it to the Manager.  The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileContinuousApply{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("continuousapply-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ContinuousApply
	err = c.Watch(&source.Kind{Type: &applyv1beta1.ContinuousApply{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileContinuousApply{}

// ReconcileContinuousApply reconciles a ContinuousApply object
type ReconcileContinuousApply struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ContinuousApply object and makes changes based on the state read
// and what is in the ContinuousApply.Spec
func (r *ReconcileContinuousApply) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	fmt.Printf("Reconcile: %v\n", request)
	// Fetch the ContinuousApply instance
	instance := &applyv1beta1.ContinuousApply{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	applier := &appsv1.Deployment{}
	spec := appsv1.DeploymentSpec{}
	applier.Labels = map[string]string{"apply.k8s.io/applier": instance.Name}
	spec.Template.Labels = applier.Labels
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: applier.Labels,
	}

	args := []string{
		"apply-issues",
		"--owner", instance.Spec.Repo.Owner,
		"--repo", instance.Spec.Repo.Repo,
		"--user", instance.Spec.User,
		"--name", instance.Name,
	}

	if instance.Spec.Type != "" {
		args = append(args, "--sync-type", instance.Spec.Type)
	}
	if instance.Spec.RolloutType != "" {
		args = append(args, "--rollout-type", instance.Spec.RolloutType)
	}
	for _, t := range instance.Spec.Targets {
		args = append(args, "--apply-targets", t.Path)
	}

	if instance.Spec.Match.Milestone != "" {
		args = append(args, "--match-milestone", instance.Spec.Match.Milestone)
	}
	if instance.Spec.Match.State != "" {
		args = append(args, "--match-state", instance.Spec.Match.State)
	}
	if instance.Spec.Match.Assignee != "" {
		args = append(args, "--match-assignee", instance.Spec.Match.Assignee)
	}
	for _, l := range instance.Spec.Match.Labels {
		args = append(args, "--match-labels", l)
	}

	if instance.Spec.BeforeActions.SetState != "" {
		args = append(args, "--before-set-state", instance.Spec.BeforeActions.SetState)
	}
	for _, a := range instance.Spec.BeforeActions.AddAssignees {
		args = append(args, "--before-add-assignees", a)
	}
	for _, l := range instance.Spec.BeforeActions.AddLabels {
		args = append(args, "--before-add-labels", l)
	}
	for _, a := range instance.Spec.BeforeActions.RemoveAssignees {
		args = append(args, "--before-remove-assignees", a)
	}
	for _, l := range instance.Spec.BeforeActions.RemoveLabels {
		args = append(args, "--before-remove-labels", l)
	}

	if instance.Spec.AfterActions.SetState != "" {
		args = append(args, "--after-set-state", instance.Spec.AfterActions.SetState)
	}
	for _, a := range instance.Spec.AfterActions.AddAssignees {
		args = append(args, "--after-add-assignees", a)
	}
	for _, l := range instance.Spec.AfterActions.AddLabels {
		args = append(args, "--after-add-labels", l)
	}
	for _, a := range instance.Spec.AfterActions.RemoveAssignees {
		args = append(args, "--after-remove-assignees", a)
	}
	for _, l := range instance.Spec.AfterActions.RemoveLabels {
		args = append(args, "--after-remove-labels", l)
	}

	automount := true
	spec.Template.Spec.ServiceAccountName = instance.Spec.Components.ServiceAccount
	spec.Template.Spec.AutomountServiceAccountToken = &automount

	// TODO: Don't hardcode the container name.
	spec.Template.Spec.Containers = []corev1.Container{
		{Name: "applier",
			Command: []string{"./continuous-apply"},
			Args:    args,
			Image:   caImage,
			Env: []corev1.EnvVar{
				{Name: "GIT_ACCESS_TOKEN", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: instance.Spec.Components.GitCredentials.Secret,
						Key:                  instance.Spec.Components.GitCredentials.Key,
					},
				}},
			},
			ImagePullPolicy: corev1.PullAlways,
		},
	}

	if instance.Spec.Components.Applier == nil {
		// TODO: Search for Deployment using label selectors also in case the second write fails
		// Create the applier object
		applier.Spec = spec
		applier.GenerateName = fmt.Sprintf("%s-", instance.Name)
		applier.Namespace = instance.Namespace
		if err := controllerutil.SetControllerReference(instance, applier, r.scheme); err != nil {
			fmt.Printf("failed to set reference %v\n", err)
			return reconcile.Result{}, err
		}

		fmt.Printf("Creating deployment\n")
		if err := r.Create(context.Background(), applier); err != nil {
			fmt.Printf("failed to update %v\n", err)
			return reconcile.Result{}, err
		}

		// Update the instance with the object reference
		instance.Spec.Components.Applier = &corev1.ObjectReference{
			Namespace:       applier.Namespace,
			Name:            applier.Name,
			Kind:            "Deployment",
			APIVersion:      "apps/v1",
			UID:             applier.UID,
			ResourceVersion: applier.ResourceVersion,
		}
		fmt.Printf("Updating applier\n")
		if err := r.Update(context.Background(), instance); err != nil {
			fmt.Printf("failed to update %v\n", err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if err := r.Client.Get(context.Background(),
		types.NamespacedName{Name: instance.Spec.Components.Applier.Name,
			Namespace: instance.Spec.Components.Applier.Namespace,
		}, applier); err != nil {
		fmt.Printf("failed to find applier %v\n", err)
		return reconcile.Result{}, err
	}
	applier.Spec = spec
	fmt.Printf("Updating applier\n")
	if err := r.Update(context.Background(), applier); err != nil {
		fmt.Printf("failed to update %v\n", err)
		return reconcile.Result{}, err
	}

	// TODO: Update the deployment args

	return reconcile.Result{}, nil
}
