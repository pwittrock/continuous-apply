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

package poller

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"os/exec"

	"strings"

	"os"

	"github.com/google/go-github/github"
	"github.com/pwittrock/continuous-apply/pkg/applier"
	"github.com/pwittrock/continuous-apply/pkg/git"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type Poller struct {
	Owner string
	Repo  string

	Issue  *github.Issue
	Pr     *github.PullRequest
	Commit string

	Type string

	// Applier to use
	applier.Applier

	// MatchLabels filters Issues/PRs to rollout by labels
	MatchLabels []string

	// MatchAssignee filters Issues/PRs to rollout by assignee
	MatchAssignee string

	// MatchMilestone filters Issues/PRs to rollout by milestone
	MatchMilestone string

	// MatchState filters Issues/PRs to rollout by state
	MatchState string

	// SkipAddLabels are labels to add when skipping an issue or Pr
	SkipAddLabels []string

	// SkipSetState is the state to set when skipping an issue or Pr
	SkipSetState string
}

var rolloutRegex = regexp.MustCompile("\\[pull-request\\]: #(\\d+)\\s+\\[commit\\]: ([a-z0-9]+)\\s+")

func (p *Poller) SyncIssues() (bool, error) {
	issues, _, err := p.GitClient.Issues.ListByRepo(context.TODO(), p.Owner, p.Repo, &github.IssueListByRepoOptions{
		State:     p.MatchState,
		Labels:    p.MatchLabels,
		Assignee:  p.MatchAssignee,
		Milestone: p.MatchMilestone,
		Sort:      "created",
		Direction: "desc",
	})
	if err != nil {
		return false, err
	}

	if len(issues) == 0 {
		return false, fmt.Errorf("no matching issues found")
	}
	log.Printf("found %d issues\n", len(issues))

	for _, i := range issues {
		fmt.Printf("issue: %d\n", i.GetNumber())
	}

	var issue *github.Issue
	closed := "closed"
	newIssue := true
	for i := range issues {
		curr := issues[i]

		// Done for first PR
		if i == 0 {
			issue = issues[i]

			// Already the current issue, do nothing
			if p.Issue != nil && p.Issue.GetID() == curr.GetID() {
				newIssue = false
				fmt.Printf("current issue %d unchanged\n", p.Issue.GetNumber())
				break
			}

			log.Printf("syncing issue %d\n", issue.GetNumber())

			// Set the new issue
			p.Issue = issue

			// Verify the issue is correctly formatted
			if !rolloutRegex.MatchString(curr.GetBody()) {
				return false, fmt.Errorf("Expected %s prefix in %d IssueNum body, got %s",
					rolloutRegex, *curr.Number, curr.GetBody())
			}

			// Parse the PR out of the issue
			matches := rolloutRegex.FindStringSubmatch(curr.GetBody())

			// Get the PR
			prNum, err := strconv.Atoi(matches[1])
			if err != nil {
				return false, err
			}
			pr, _, err := p.GitClient.PullRequests.Get(context.TODO(), p.Owner, p.Repo, prNum)
			if err != nil {
				return false, err
			}
			p.Pr = pr

			// Get the commit
			commit := matches[2]
			p.Commit = commit

			log.Printf("syncing commit %s\n", p.Commit)
			continue
		}
	}

	// IssueNum is done
	if p.Issue.GetState() == closed {
		return false, nil
	}

	return newIssue, nil
}

var prRegex = regexp.MustCompile("^([a-z0-9]+) .*Merge pull request #(\\d+) from ")

func (p *Poller) SyncPRs() (bool, error) {
	log.Printf("fetching git\n")
	cmd := exec.Command("git", "fetch", "origin")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("could not fetch git %v", err)
	}

	cmd = exec.Command("git", "log", "origin/master", "--merges", "--pretty=oneline")
	o, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("could not retrieve git log %v", err)
	}

	for _, l := range strings.Split(string(o), "\n") {
		matches := prRegex.FindStringSubmatch(string(l))
		if len(matches) < 3 {
			continue
		}
		commit := matches[1]
		num, err := strconv.Atoi(matches[2])
		if err != nil {
			continue
		}

		// No new issues
		if commit == p.Commit {
			return false, nil
		}

		// New issue found
		log.Printf("Finding commit %s for PR %d\n", commit, num)
		issue, _, err := p.GitClient.Issues.Get(context.Background(), p.Owner, p.Repo, num)
		if err != nil {
			return false, err
		}

		// Filter PRs
		if len(p.MatchLabels) > 0 {
			labels := sets.String{}
			for _, l := range issue.Labels {
				labels.Insert(l.GetName())
			}
			match := true
			for _, l := range p.MatchLabels {
				if !labels.Has(l) {
					match = false
					fmt.Printf("label %s missing from %v\n", l, labels.List())
					break
				}
			}
			if !match {
				continue
			}
		}
		if p.MatchAssignee != "" {
			found := false
			for _, a := range issue.Assignees {
				if a.GetLogin() == p.MatchAssignee {
					found = true
				}
			}
			if !found {
				fmt.Printf("assignee %s not found\n", p.MatchAssignee)
				continue
			}
		}
		if p.MatchMilestone != "" {
			if issue.Milestone.GetTitle() != p.MatchMilestone {
				fmt.Printf("milestone %s does not match\n", issue.Milestone.GetTitle())
				continue
			}
		}
		if p.MatchState != "" {
			if issue.GetState() != p.MatchState && p.MatchState != "all" {
				fmt.Printf("state %s does not match\n", issue.GetState())
				continue
			}
		}

		p.Issue = issue
		pr, _, err := p.GitClient.PullRequests.Get(context.TODO(), p.Owner, p.Repo, num)
		if err != nil {
			return false, err
		}
		p.Pr = pr
		p.Commit = commit
		p.IssueNum = num
		return true, nil
	}
	return false, fmt.Errorf("no matching PRs found")
}

func (p *Poller) Apply() error {
	var err error
	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	p.Applier.K8sClient, err = client.New(cfg, client.Options{})
	if err != nil {
		return err
	}

	if p.Applier.GitClient, err = git.NewManager(p.Owner, p.Repo, p.Commit); err != nil {
		return err
	}
	p.Applier.IssueNum = int(p.Issue.GetNumber())
	if err != nil {
		return err
	}
	return p.Applier.Run()
}

func (p *Poller) Run() error {
	if err := p.GitClient.Clone(); err != nil {
		return err
	}

	fmt.Printf("running")
	for {
		// Find the commit and issue
		var newCommit bool
		var err error
		switch strings.TrimSpace(strings.ToLower(p.Type)) {
		case "pr":
			// Look for PRs
			newCommit, err = p.SyncPRs()
			if err != nil {
				log.Printf("%v", err)
				time.Sleep(30 * time.Second)
				continue
			}
		case "issue", "":
			// Look for issues
			newCommit, err = p.SyncIssues()
			if err != nil {
				log.Printf("%v", err)
				time.Sleep(30 * time.Second)
				continue
			}
		}
		// Commit wasn't changed, do nothing
		if !newCommit {
			time.Sleep(30 * time.Second)
			continue
		}

		// Apply the most recent issue
		if err := p.Apply(); err != nil {
			log.Printf("%v", err)
		}
		time.Sleep(30 * time.Second)
	}
}
