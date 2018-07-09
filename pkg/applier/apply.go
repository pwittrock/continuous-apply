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

package applier

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/google/go-github/github"
	"github.com/pwittrock/continuous-apply/pkg/git"
	"github.com/pwittrock/continuous-apply/pkg/rollout"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Applier struct {
	K8sClient client.Client
	GitClient *git.GitManager
	Name      string

	ApplyTargets []string

	// User is the GitHub use we are acting as
	User string

	// IssueNum is the GitHub issue number to update with the rollout status
	IssueNum int

	// BeforeAddLabels are labels to add before starting the rollout
	BeforeAddLabels []string

	// BeforeAddAssignees are assignees to add before starting the rollout
	BeforeAddAssignees []string

	// BeforeRemoveLabels are labels to remove before starting the rollout
	BeforeRemoveLabels []string

	// BeforeRemoveAssignees are assignees to remove before starting the rollout
	BeforeRemoveAssignees []string

	// BeforeSetState is the state to set before starting the rollout
	BeforeSetState string

	// AfterAddLabels are labels to add after completing the rollout
	AfterAddLabels []string

	// AfterAddAssignees are assignees to add after completing the rollout
	AfterAddAssignees []string

	// AfterRemoveLabels are labels to remove after completing the rollout
	AfterRemoveLabels []string

	// AfterRemoveAssignees are assignees to remove after completing the rollout
	AfterRemoveAssignees []string

	// AfterSetState is the state to set after completing the rollout
	AfterSetState string

	// Pause is the time to wait between checking status of a rollout
	Pause time.Duration

	RolloutType string
}

const doneIcon = "![done](https://material.io/tools/icons/static/icons/twotone-done-24px.svg)"
const inProgressIcon = "![inprogress](https://material.io/tools/icons/static/icons/twotone-cached-24px.svg)"

func (a *Applier) beforeActions() error {
	if len(a.BeforeAddLabels) > 0 {
		if err := a.GitClient.AddLabels(a.IssueNum, a.BeforeAddLabels...); err != nil {
			return fmt.Errorf("failed to add labels %v %v", a.BeforeAddLabels, err)
		}
	}
	// Don't fail in case the labels aren't already set
	_ = a.GitClient.RemoveLabels(a.IssueNum, a.BeforeRemoveLabels...)

	if len(a.BeforeAddAssignees) > 0 {
		if err := a.GitClient.AddAssignees(a.IssueNum, a.BeforeAddAssignees...); err != nil {
			return fmt.Errorf("failed to add labels %v %v", a.BeforeAddAssignees, err)
		}
	}
	// Don't fail in case the assignees aren't already set
	_ = a.GitClient.RemoveAssignees(a.IssueNum, a.BeforeRemoveAssignees...)

	if a.BeforeSetState != "" {
		if err := a.GitClient.UpdateIssueState(a.IssueNum, a.BeforeSetState); err != nil {
			return fmt.Errorf("failed to set state %v", err)
		}
	}

	return nil
}

func (a *Applier) afterActions() error {
	if len(a.AfterAddLabels) > 0 {
		if err := a.GitClient.AddLabels(a.IssueNum, a.AfterAddLabels...); err != nil {
			return err
		}
	}

	// Don't fail if the labels don't exist
	_ = a.GitClient.RemoveLabels(a.IssueNum, a.AfterRemoveLabels...)

	if len(a.AfterAddAssignees) > 0 {
		if err := a.GitClient.AddAssignees(a.IssueNum, a.AfterAddAssignees...); err != nil {
			return fmt.Errorf("failed to set assignees %v %v", a.AfterAddAssignees, err)
		}
	}

	// Don't fail if the assignees don't exist
	_ = a.GitClient.RemoveAssignees(a.IssueNum, a.AfterRemoveAssignees...)

	if a.AfterSetState != "" {
		if err := a.GitClient.UpdateIssueState(a.IssueNum, a.AfterSetState); err != nil {
			return fmt.Errorf("failed to set issue state %v", err)
		}
	}
	return nil
}

func (a *Applier) Run() error {
	// Sync the repo
	if err := a.GitClient.SyncRepo(); err != nil {
		return err
	}

	log.Printf("Repo synced\n")

	// Do GitHub actions
	if err := a.beforeActions(); err != nil {
		return err
	}

	// Get the GH comment to update with the Status
	comment, err := a.GitClient.GetComment(a.Name, a.User, a.IssueNum)
	if err != nil {
		return err
	}

	ros := &rollout.Rollouts{
		Status: "In Progress",
		Name:   a.Name,
		Icon:   inProgressIcon,
	}
	for _, path := range a.ApplyTargets {
		log.Printf("kustomizing %s\n", path)

		// Kustomize the objects
		objects, err := a.kustomize(path)
		if err != nil {
			return err
		}

		log.Printf("adding %d items to rollout\n", len(objects))

		// Get each of the rollouts
		ro, err := a.getRollout(comment, objects...)
		if err != nil {
			return err
		}
		ro.Path = path
		ros.Rollouts = append(ros.Rollouts, ro)
	}

	if _, err := a.updateComment(comment, ros); err != nil {
		return err
	}

	if a.RolloutType == "sequential" || a.RolloutType == "" {
		for _, ro := range ros.Rollouts {
			if err := a.applyAllSequential(comment, ro, ros); err != nil {
				return err
			}
		}
	} else {
		if err := a.applyAllParallel(comment, ros); err != nil {
			return err
		}
	}
	ros.Status = "Complete"
	ros.Icon = doneIcon
	if comment, err = a.updateComment(comment, ros); err != nil {
		return err
	}

	// Do GitHub actions
	if err := a.afterActions(); err != nil {
		return err
	}
	return nil
}

func (a *Applier) kustomize(path string) ([]string, error) {
	out, err := exec.Command("kustomize", "build", path).CombinedOutput()
	if err != nil {
		log.Printf("failed to kustomize %s\n", out)
		return nil, err
	}
	return strings.Split(string(out), "---\n"), nil
}

func (a *Applier) getRollout(comment *github.IssueComment, objects ...string) (*rollout.Rollout, error) {
	// Parse each of the objects and add them to the list
	ro := &rollout.Rollout{
		Status: "Pending",
	}
	for _, o := range objects {
		obj, err := rollout.ParseObject([]byte(o))
		if err != nil {
			return ro, err
		}
		ro.Objects = append(ro.Objects, obj)
	}

	return ro, nil
}

func (a *Applier) applyAllParallel(comment *github.IssueComment, ros *rollout.Rollouts) error {
	for _, ro := range ros.Rollouts {
		ro.Status = "In Progress"
		ro.Icon = inProgressIcon
		for _, o := range ro.Objects {
			log.Printf("applying %s\n", o.Display())
			// Apply the object
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewBuffer(o.Raw)
			out, err := cmd.CombinedOutput()
			log.Printf("%s", out)
			o.ApplyStatus = strings.TrimSpace(string(out))
			if err != nil {
				_, _ = a.updateComment(comment, ros)
				return fmt.Errorf("%v error applying %s", err, o.Raw)
			}
		}
	}

	var err error
	if comment, err = a.updateComment(comment, ros); err != nil {
		return err
	}

	done := false
	for !done {
		done = true
		rodone := true
		for _, ro := range ros.Rollouts {
			for _, o := range ro.Objects {
				// Wait for rollout to complete
				viewer := rollout.GetStatusViewer(o.Object, a.K8sClient)
				if viewer == nil {
					o.RolloutStatus = "NA"
					o.Done = true
					continue
				}

				status, d, err := viewer.Status(o.NamespacedName, 0)
				status = strings.TrimSpace(status)
				o.Done = done

				if err != nil {
					o.RolloutStatus = fmt.Sprintf("error: %v", err)
					_, _ = a.updateComment(comment, ros)
					return fmt.Errorf("'%v' error getting rollout status for %s\n%T - %s %s",
						err, o.JSON, o.Object, o.Name, o.Namespace)
				}

				if o.RolloutStatus != status {
					log.Println(status)
					o.RolloutStatus = status
					o.RolloutStatusHistory = append(o.RolloutStatusHistory, fmt.Sprintf("*%s* - `%s`", time.Now().Format(time.RFC822), status))
					if comment, err = a.updateComment(comment, ros); err != nil {
						return err
					}
				}

				// Pause between checking status
				if !d {
					done = false
					rodone = false
				}
			}
			if rodone {
				ro.Status = "Complete"
				ro.Icon = doneIcon
			}
		}
		if comment, err = a.updateComment(comment, ros); err != nil {
			return err
		}
		if !done {
			time.Sleep(a.Pause)
		}
	}

	return nil
}

func (a *Applier) applyAllSequential(comment *github.IssueComment, ro *rollout.Rollout, ros *rollout.Rollouts) error {
	ro.Status = "In Progress"
	ro.Icon = inProgressIcon

	for _, o := range ro.Objects {
		log.Printf("applying %s\n", o.Display())
		// Apply the object
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = bytes.NewBuffer(o.Raw)
		out, err := cmd.CombinedOutput()
		log.Printf("%s", out)
		o.ApplyStatus = strings.TrimSpace(string(out))
		if err != nil {
			_, _ = a.updateComment(comment, ros)
			return fmt.Errorf("%v error applying %s", err, o.Raw)
		}
	}

	var err error
	if comment, err = a.updateComment(comment, ros); err != nil {
		return err
	}

	done := false
	for !done {
		done = true
		for _, o := range ro.Objects {
			// Wait for rollout to complete
			viewer := rollout.GetStatusViewer(o.Object, a.K8sClient)
			if viewer == nil {
				o.RolloutStatus = "NA"
				o.Done = true
				continue
			}

			status, d, err := viewer.Status(o.NamespacedName, 0)
			status = strings.TrimSpace(status)
			o.Done = d

			if err != nil {
				o.RolloutStatus = fmt.Sprintf("error: %v", err)
				_, _ = a.updateComment(comment, ros)
				return fmt.Errorf("'%v' error getting rollout status for %s\n%T - %s %s",
					err, o.JSON, o.Object, o.Name, o.Namespace)
			}

			if o.RolloutStatus != status {
				log.Println(status)
				o.RolloutStatus = status
				o.RolloutStatusHistory = append(o.RolloutStatusHistory, fmt.Sprintf("*%s* - `%s`", time.Now().Format(time.RFC822), status))
				if comment, err = a.updateComment(comment, ros); err != nil {
					return err
				}
			}

			// Pause between checking status
			if !d {
				done = false
			}
		}
		if comment, err = a.updateComment(comment, ros); err != nil {
			return err
		}
		if !done {
			time.Sleep(a.Pause)
		}
	}
	ro.Status = "Complete"
	ro.Icon = doneIcon

	return nil
}

func (a *Applier) updateComment(comment *github.IssueComment, ro *rollout.Rollouts) (*github.IssueComment, error) {
	// Execute the template
	b := &bytes.Buffer{}
	if err := issueTemplate.Execute(b, ro); err != nil {
		return nil, err
	}

	// Update the comment
	body := b.String()
	comment.Body = &body
	return a.GitClient.UpdateComment(comment, a.Name, a.User, a.IssueNum)
}

const issueTemplateBody = `
## {{ .Icon }} {{ .Name }} - *{{ .Status }}*
---

{{range $ro := .Rollouts }}### {{ $ro.Icon }} ` + "`{{ $ro.Path }}`" + ` - *{{ $ro.Status }}*

{{ range $obj := $ro.Objects }}
- [{{ if $obj.Done}}x{{else}} {{end}}] {{ $obj.Display }}
{{ if $obj.ApplyStatus }}  - ` + "**apply:** `{{ $obj.ApplyStatus}}`" + `
{{ end -}}
{{ if $obj.RolloutStatus }}  - ` + "**rollout:** `{{ $obj.RolloutStatus}}`" + `
{{range $h := $obj.RolloutStatusHistory }}    - {{ $h }}
{{ end }}
{{ end -}}
{{ end }}
---
{{ end }}
`

var issueTemplate = template.Must(template.New("comment").Parse(issueTemplateBody))
