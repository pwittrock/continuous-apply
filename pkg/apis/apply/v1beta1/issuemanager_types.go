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

// IssueManagerSpec defines the desired state of IssueManager
type IssueManagerSpec struct {
	// Repo is the Git repo to pull from.
	Repo Repo `json:"repo"`

	// User is the GitHub user that will comment on the PR
	User string `json:"user"`

	// OpenIssue filters which PRs to open issues for.
	OpenIssue GitMatch `json:"openIssue"`

	// OpenActions are actions performed after opening an issue.
	OpenActions GitActions `json:"openActions"`

	// CloseIssue filters which issues to close.
	CloseIssue GitMatch `json:"closeIssue"`

	// CloseActions are actions performed after closing an issue.
	CloseActions GitActions `json:"closeActions"`

	// Components are dependencies - e.g. GitHub credentials and the ServiceAccount to apply with
	Components IssueManagerComponents `json:"components"`

	StatusReporters []*StatusReporters `json:"statusReporters"`

	Label string `json:"label"`
}

type StatusReporters struct {
	Name string `json:"name"`

	Status     string
	StatusIcon string
	Done       bool

	InProgressLabels []string `json:"inProgressLabels"`

	CompleteLabels []string `json:"completeLabels"`

	WaitFor []string `json:"waitFor"`
}

type IssueManagerComponents struct {
	// GitCredentials is a reference to the Secret containing the GitCredentials
	GitCredentials `json:"gitCredentials"`

	// +optional
	IssueManager *v1.ObjectReference `json:"issueManager,omitempty"`
}

// IssueManagerStatus defines the observed state of IssueManager
type IssueManagerStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IssueManager is the Schema for the issuemanagers API
// +k8s:openapi-gen=true
type IssueManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IssueManagerSpec   `json:"spec,omitempty"`
	Status IssueManagerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IssueManagerList contains a list of IssueManager
type IssueManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IssueManager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IssueManager{}, &IssueManagerList{})
}
