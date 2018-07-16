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

package issues

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/pwittrock/continuous-apply/pkg/apis/apply/v1beta1"
	"github.com/pwittrock/continuous-apply/pkg/git"
	"github.com/pwittrock/continuous-apply/pkg/poller"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Manager struct {
	v1beta1.IssueManagerSpec
	AccessToken string

	gitHubClient *github.Client
	gitClient    *git.GitManager
	poller       *poller.Poller
	Issue        *github.Issue
	PullRequest  *github.PullRequest
	Commit       string
}

func (m *Manager) Run() error {
	token := strings.TrimSpace(m.AccessToken)
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	m.gitHubClient = github.NewClient(oauth2.NewClient(ctx, ts))

	m.gitClient = &git.GitManager{
		Owner:       m.Repo.Owner,
		Repo:        m.Repo.Repo,
		Client:      m.gitHubClient,
		AccessToken: m.AccessToken,
	}
	m.poller = &poller.Poller{
		Repo:           m.Repo.Repo,
		Owner:          m.Repo.Owner,
		MatchState:     m.OpenIssue.State,
		MatchLabels:    m.OpenIssue.Labels,
		MatchAssignee:  m.OpenIssue.Assignee,
		MatchMilestone: m.OpenIssue.Milestone,
	}
	m.poller.Applier.GitClient = m.gitClient

	if err := m.gitClient.Clone(); err != nil {
		return err
	}

	for {
		if err := m.SyncToPRAndIssue(); err != nil {
			return err
		}
		if err := m.UpdateIssueStatus(); err != nil {
			return err
		}
		time.Sleep(30 * time.Second)
	}
}

const doneIcon = "![done](https://material.io/tools/icons/static/icons/twotone-done-24px.svg)"
const inProgressIcon = "![inprogress](https://material.io/tools/icons/static/icons/twotone-cached-24px.svg)"

func (m *Manager) UpdateIssueStatus() error {
	var err error
	log.Printf("checking issue %d\n", m.Issue.GetNumber())
	m.Issue, _, err = m.gitHubClient.Issues.Get(context.Background(), m.Repo.Owner, m.Repo.Repo, m.Issue.GetNumber())
	if err != nil {
		return err
	}
	s := sets.NewString()
	for _, l := range m.Issue.Labels {
		s.Insert(l.GetName())
	}
	state := ""
	for _, t := range m.StatusReporters {
		switch {
		case s.HasAll(t.CompleteLabels...):
			log.Printf("%s Complete\n", t.Name)
			t.Status = "Complete"
			t.Done = true
			t.StatusIcon = doneIcon
			state = "closed"
		case s.HasAll(t.InProgressLabels...):
			log.Printf("%s In Progress\n", t.Name)
			t.Status = "In Progress"
			t.Done = false
			t.StatusIcon = inProgressIcon
			state = "open"
		default:
			log.Printf("%s Pending\n", t.Name)
			t.Status = "Pending"
			t.Done = false
			t.StatusIcon = ""
			state = "open"
		}
	}

	buff := &bytes.Buffer{}
	if err := bodyTemplate.Execute(buff, m); err != nil {
		return err
	}
	b := buff.String()
	m.Issue, _, err = m.gitHubClient.Issues.Edit(context.Background(), m.Repo.Owner, m.Repo.Repo, m.Issue.GetNumber(),
		&github.IssueRequest{
			State: &state,
			Body:  &b,
		})
	return err
}

var rolloutRegex = regexp.MustCompile("\\[pull-request\\]: #(\\d+)\\s+\\[commit\\]: ([a-z0-9]+)\\s+")

func (m *Manager) SyncToPRAndIssue() error {
	m.PullRequest = nil
	m.Commit = ""
	m.Issue = nil

	// Get the PR we should sync next
	if err := m.SyncPollerPRs(); err != nil {
		return err
	}
	m.PullRequest = m.poller.Pr
	m.Commit = m.poller.Commit

	issues, _, err := m.gitHubClient.Issues.ListByRepo(context.Background(), m.Repo.Owner, m.Repo.Repo, &github.IssueListByRepoOptions{
		Labels:    []string{m.Label},
		Assignee:  m.OpenIssue.Assignee,
		Milestone: m.OpenIssue.Milestone,
		State:     "all",
		Sort:      "created",
		Direction: "desc",
	})
	if err != nil {
		return err
	}

	first := true
	log.Printf("Checking issues %s\n", m.Label)
	// Find matching issue if it exists already
	for _, issue := range issues {
		log.Printf("Checking issue %d\n", issue.GetNumber())
		if issue.IsPullRequest() {
			continue
		}
		if first {
			body := issue.GetBody()
			if !rolloutRegex.MatchString(body) {
				log.Printf("Expected \"[pull-request]: #(\\d+)\n[commit]: ([a-z0-9]+)\n\" "+
					"prefix in %d IssueNum body, got %s", issue.GetNumber(), body)
			}

			matches := rolloutRegex.FindStringSubmatch(body)
			if len(matches) != 3 {
				log.Printf("could not match pull request and commit %s", body)
				continue
			}

			prNum, err := strconv.Atoi(matches[1])
			if err != nil {
				log.Printf("could not parse PullRequest %v", err)
				continue
			}
			commit := matches[2]

			fmt.Printf("Found PR %s %d\n", m.Commit, m.PullRequest.GetNumber())
			fmt.Printf("Found Issue %s %d\n", commit, prNum)

			first = false
			if prNum != m.PullRequest.GetNumber() {
				log.Printf("PR does not match %d %d", prNum, m.PullRequest.GetNumber())
				closed := "closed"
				_, _, err := m.gitHubClient.Issues.Edit(context.Background(), m.Repo.Owner, m.Repo.Repo, issue.GetNumber(),
					&github.IssueRequest{
						State: &closed,
					})
				if err != nil {
					log.Printf("could not close issue %v %v", issue.GetNumber(), err)
				}
				continue
			}
			if commit != m.Commit {
				log.Printf("Commit does not match [%s] [%s]", commit, m.Commit)
				closed := "closed"
				_, _, err := m.gitHubClient.Issues.Edit(context.Background(), m.Repo.Owner, m.Repo.Repo, issue.GetNumber(),
					&github.IssueRequest{
						State: &closed,
					})
				if err != nil {
					log.Printf("could not close issue %v %v", issue.GetNumber(), err)
				}
				continue
			}

			m.Issue = issue
			log.Printf("Issue %d matches PR %d\n", m.Issue.GetNumber(), m.PullRequest.GetNumber())
		} else {
			closed := "closed"
			_, _, err := m.gitHubClient.Issues.Edit(context.Background(), m.Repo.Owner, m.Repo.Repo, issue.GetNumber(),
				&github.IssueRequest{
					State: &closed,
				})
			if err != nil {
				log.Printf("could not close issue %v %v", issue.GetNumber(), err)
			}
		}
	}

	// Create a new issue
	if m.Issue == nil {
		log.Printf("nil issue for %d %s\n", m.PullRequest.GetNumber(), m.Commit)
		if m.Commit == "" {
			return fmt.Errorf("no commit for PR")
		}

		if m.PullRequest == nil {
			return fmt.Errorf("no PullRequest")
		}

		// Create a new issue for the PR
		o := &bytes.Buffer{}
		if err = bodyTemplate.Execute(o, m); err != nil {
			return err
		}
		issueBody := o.String()
		title := fmt.Sprintf("Rollout #%d", m.PullRequest.GetNumber())
		labels := append(m.OpenActions.AddLabels, m.Label)
		ir := &github.IssueRequest{
			Body:   &issueBody,
			Labels: &labels,
			Title:  &title,
		}
		if len(m.OpenActions.AddAssignees) > 0 {
			ir.Assignees = &m.OpenActions.AddAssignees
		}
		if m.Issue, _, err = m.gitHubClient.Issues.Create(context.Background(), m.Repo.Owner, m.Repo.Repo, ir); err != nil {
			log.Printf("could not open issue %v", err)
		}

		//if _, _, err = m.gitHubClient.Issues.ReplaceLabelsForIssue(
		//	context.Background(), m.Repo.Owner, m.Repo.Repo, m.Issue.GetNumber(), *ir.Labels); err != nil {
		//	log.Printf("could not open issue %v", err)
		//}
		log.Printf("opened issue %d\n", m.Issue.GetNumber())
	}

	return nil
}

var bodyTemplate = template.Must(template.New("name").Parse(`[pull-request]: #{{ .PullRequest.GetNumber}}
[commit]: {{ .Commit }}

Rollout #{{ .PullRequest.GetNumber}}

{{ range $r := .StatusReporters -}}	
- {{ $r.StatusIcon }} {{ $r.Name }} - *{{ $r.Status }}*{{ if not $r.Done }}{{ if $r.WaitFor}} (run after{{ range $w := $r.WaitFor }} {{ $w }}{{ end }}){{end}}{{ end }}
{{ end -}}
`))

func (m *Manager) SyncPollerPRs() error {
	found := false
	for !found {
		var err error
		if found, err = m.poller.SyncPRs(); err != nil {
			return err
		}
		if !found {
			time.Sleep(30 * time.Second)
		}
	}
	return nil
}
