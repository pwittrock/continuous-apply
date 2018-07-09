/*
Copyright 2018 The Kubernetes Authors.

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

package rollout

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	appsv1beta2 "k8s.io/api/apps/v1beta2"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatusViewer provides an interface for resources that have rollout status.
type StatusViewer interface {
	Status(name types.NamespacedName, revision int64) (string, bool, error)
}

func GetStatusViewer(o runtime.Object, c client.Client) StatusViewer {
	switch o.(type) {
	// Deployment cases
	case *extensionsv1beta1.Deployment:
		return &DeploymentStatusViewer{Client: c}
	case *appsv1beta1.Deployment:
		return &DeploymentStatusViewer{Client: c}
	case *appsv1beta2.Deployment:
		return &DeploymentStatusViewer{Client: c}
	case *appsv1.Deployment:
		return &DeploymentStatusViewer{Client: c}

	// StatefulSet cases
	case *appsv1beta1.StatefulSet:
		return &StatefulSetStatusViewer{Client: c}
	case *appsv1beta2.StatefulSet:
		return &StatefulSetStatusViewer{Client: c}
	case *appsv1.StatefulSet:
		return &StatefulSetStatusViewer{Client: c}

	// DaemonSet cases
	case *extensionsv1beta1.DaemonSet:
		return &DaemonSetStatusViewer{Client: c}
	case *appsv1beta2.DaemonSet:
		return &DaemonSetStatusViewer{Client: c}
	case *appsv1.DaemonSet:
		return &DaemonSetStatusViewer{Client: c}

	case *unstructured.Unstructured:
		return nil
	default:
		// no match; here v has the same type as i
	}

	return nil
}

func setNs(v v1.Object) string {
	if v.GetNamespace() == "" {
		v.SetNamespace("default")
	}
	return v.GetNamespace()
}

// DeploymentStatusViewer implements the StatusViewer interface.
type DeploymentStatusViewer struct {
	client.Client
}

// DaemonSetStatusViewer implements the StatusViewer interface.
type DaemonSetStatusViewer struct {
	client.Client
}

// StatefulSetStatusViewer implements the StatusViewer interface.
type StatefulSetStatusViewer struct {
	client.Client
}

// Status returns a message describing deployment status, and a bool value indicating if the status is considered done.
func (s *DeploymentStatusViewer) Status(name types.NamespacedName, revision int64) (string, bool, error) {
	deployment := &appsv1.Deployment{}
	err := s.Get(context.TODO(), name, deployment)
	if err != nil {
		return "", false, err
	}
	if revision > 0 {
		deploymentRev, err := Revision(deployment)
		if err != nil {
			return "", false, fmt.Errorf("cannot get the revision of deployment %q: %v", deployment.Name, err)
		}
		if revision != deploymentRev {
			return "", false, fmt.Errorf("desired revision (%d) is different from the running revision (%d)", revision, deploymentRev)
		}
	}
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		cond := GetDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
		if cond != nil && cond.Reason == TimedOutReason {
			return "", false, fmt.Errorf("deployment %q exceeded its progress deadline", name)
		}
		if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d out of %d new replicas have been updated...\n", name, deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas), false, nil
		}
		if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d old replicas are pending termination...\n", name, deployment.Status.Replicas-deployment.Status.UpdatedReplicas), false, nil
		}
		if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d of %d updated replicas are available...\n", name, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas), false, nil
		}
		return fmt.Sprintf("deployment %q successfully rolled out\n", name), true, nil
	}
	return fmt.Sprintf("Waiting for deployment spec update to be observed...\n"), false, nil
}

// Status returns a message describing daemon set status, and a bool value indicating if the status is considered done.
func (s *DaemonSetStatusViewer) Status(name types.NamespacedName, revision int64) (string, bool, error) {
	//ignoring revision as DaemonSets does not have history yet

	daemon := &appsv1.DaemonSet{}
	err := s.Get(context.TODO(), name, daemon)
	if err != nil {
		return "", false, err
	}

	if daemon.Spec.UpdateStrategy.Type != appsv1.RollingUpdateDaemonSetStrategyType {
		return "", true, fmt.Errorf("Status is available only for RollingUpdate strategy type")
	}
	if daemon.Generation <= daemon.Status.ObservedGeneration {
		if daemon.Status.UpdatedNumberScheduled < daemon.Status.DesiredNumberScheduled {
			return fmt.Sprintf("Waiting for daemon set %q rollout to finish: %d out of %d new pods have been updated...\n", name, daemon.Status.UpdatedNumberScheduled, daemon.Status.DesiredNumberScheduled), false, nil
		}
		if daemon.Status.NumberAvailable < daemon.Status.DesiredNumberScheduled {
			return fmt.Sprintf("Waiting for daemon set %q rollout to finish: %d of %d updated pods are available...\n", name, daemon.Status.NumberAvailable, daemon.Status.DesiredNumberScheduled), false, nil
		}
		return fmt.Sprintf("daemon set %q successfully rolled out\n", name), true, nil
	}
	return fmt.Sprintf("Waiting for daemon set spec update to be observed...\n"), false, nil
}

// Status returns a message describing statefulset status, and a bool value indicating if the status is considered done.
func (s *StatefulSetStatusViewer) Status(name types.NamespacedName, revision int64) (string, bool, error) {
	sts := &appsv1.StatefulSet{}
	err := s.Get(context.TODO(), name, sts)
	if err != nil {
		return "", false, err
	}

	if sts.Spec.UpdateStrategy.Type == OnDeleteStatefulSetStrategyType {
		return "", true, fmt.Errorf("%s updateStrategy does not have a Status`", OnDeleteStatefulSetStrategyType)
	}
	if sts.Status.ObservedGeneration == 0 || sts.Generation > sts.Status.ObservedGeneration {
		return "Waiting for statefulset spec update to be observed...\n", false, nil
	}
	if sts.Spec.Replicas != nil && sts.Status.ReadyReplicas < *sts.Spec.Replicas {
		return fmt.Sprintf("Waiting for %d pods to be ready...\n", *sts.Spec.Replicas-sts.Status.ReadyReplicas), false, nil
	}
	if sts.Spec.UpdateStrategy.Type == RollingUpdateStatefulSetStrategyType && sts.Spec.UpdateStrategy.RollingUpdate != nil {
		if sts.Spec.Replicas != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			if sts.Status.UpdatedReplicas < (*sts.Spec.Replicas - *sts.Spec.UpdateStrategy.RollingUpdate.Partition) {
				return fmt.Sprintf("Waiting for partitioned roll out to finish: %d out of %d new pods have been updated...\n",
					sts.Status.UpdatedReplicas, (*sts.Spec.Replicas - *sts.Spec.UpdateStrategy.RollingUpdate.Partition)), false, nil
			}
		}
		return fmt.Sprintf("partitioned roll out complete: %d new pods have been updated...\n",
			sts.Status.UpdatedReplicas), true, nil
	}
	if sts.Status.UpdateRevision != sts.Status.CurrentRevision {
		return fmt.Sprintf("waiting for statefulset rolling update to complete %d pods at revision %s...\n",
			sts.Status.UpdatedReplicas, sts.Status.UpdateRevision), false, nil
	}
	return fmt.Sprintf("statefulset rolling update complete %d pods at revision %s...\n", sts.Status.CurrentReplicas, sts.Status.CurrentRevision), true, nil

}

const (
	TimedOutReason                       = "ProgressDeadlineExceeded"
	OnDeleteStatefulSetStrategyType      = "OnDelete"
	RollingUpdateStatefulSetStrategyType = "RollingUpdate"
	RevisionAnnotation                   = "deployment.kubernetes.io/revision"
)

// Revision returns the revision number of the input object.
func Revision(obj runtime.Object) (int64, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return 0, err
	}
	v, ok := acc.GetAnnotations()[RevisionAnnotation]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

func GetDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}
