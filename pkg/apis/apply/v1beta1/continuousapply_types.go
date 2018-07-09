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

package v1beta1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContinuousApplySpec defines the desired state of ContinuousApply
type ContinuousApplySpec struct {
	// Type may be "issue" or "pr".  Defaults to "pr".
	Type string `json:"type"`

	// RolloutType
	RolloutType string `json:"rolloutType"`

	// Repo is the Git repo to pull from.
	Repo Repo `json:"repo"`

	// User is the GitHub user that will comment on the PR
	User string `json:"user"`

	// Targets is a list of targets to apply.
	Targets []ApplyTarget `json:"targets"`

	// Components are dependencies - e.g. GitHub credentials and the ServiceAccount to apply with
	Components ContinuousApplyComponents `json:"components"`

	// BeforeActions are actions performed immediately before applying the commit.
	BeforeActions GitActions `json:"beforeActions"`

	// AfterActions are actions performed immediately after applying the commit.
	AfterActions GitActions `json:"afterActions"`

	// Match filters which PRs or Issues to apply.
	Match GitMatch `json:"match"`
}

type ContinuousApplyComponents struct {
	// GitCredentials is a reference to the Secret containing the GitCredentials
	GitCredentials `json:"gitCredentials"`

	// +optional
	Applier *v1.ObjectReference `json:"applier,omitempty"`

	ServiceAccount string `json:"serviceAccount"`
}

type GitCredentials struct {
	Secret v1.LocalObjectReference `json:"secret"`
	Key    string                  `json:"key"`
}

// ContinuousApplyStatus defines the observed state of ContinuousApply
type ContinuousApplyStatus struct {
	CommitSHA string `json:"commitSHA"`
	Issue     int    `json:"issue"`
}

type Repo struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type ApplyTarget struct {
	Path string `json:"path"`
}

type GitActions struct {
	// State may be one of "closed", "open"
	SetState string `json:"setState"`

	AddLabels    []string `json:"addLabels"`
	AddAssignees []string `json:"addAssignees"`

	RemoveLabels    []string `json:"removeLabels"`
	RemoveAssignees []string `json:"removeAssignees"`
}

type GitMatch struct {
	Labels    []string `json:"labels"`
	Assignee  string   `json:"assignee"`
	Milestone string   `json:"milestone"`

	// State may be one of "closed", "open"
	State string `json:"state"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ContinuousApply is the Schema for the continuousapplies API
// +k8s:openapi-gen=true
type ContinuousApply struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContinuousApplySpec   `json:"spec,omitempty"`
	Status ContinuousApplyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ContinuousApplyList contains a list of ContinuousApply
type ContinuousApplyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContinuousApply `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ContinuousApply{}, &ContinuousApplyList{})
}
