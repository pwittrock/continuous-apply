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

package git

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

const (
	tokenVarName = "GIT_ACCESS_TOKEN"
)

type GitManager struct {
	*github.Client
	AccessToken string
	Repo        string
	Owner       string
	Commit      string
}

func NewManager(owner, repo, commit string) (*GitManager, error) {
	if os.Getenv(tokenVarName) == "" {
		return nil, fmt.Errorf("must define %s environment variable", tokenVarName)
	}

	token := strings.TrimSpace(os.Getenv(tokenVarName))
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return &GitManager{
		Client:      github.NewClient(oauth2.NewClient(ctx, ts)),
		AccessToken: token,
		Commit:      commit,
		Repo:        repo,
		Owner:       owner,
	}, nil
}

func (m *GitManager) SyncRepo() error {
	if err := m.Clone(); err != nil {
		return err
	}
	if err := m.fetch(); err != nil {
		return err
	}
	if err := m.checkout(); err != nil {
		return err
	}
	return nil
}

func (m *GitManager) gitUrl() string {
	return fmt.Sprintf("https://x-accessToken-tokenVarName:%s@github.com/%s/%s", m.AccessToken, m.Owner, m.Repo)
}

func (m *GitManager) Clone() error {
	err := exec.Command("git", "remote").Run()
	if err == nil {
		return nil
	}

	log.Printf("clone started\n")

	cmd := exec.Command("git", "clone", m.gitUrl())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf(strings.Join(cmd.Args, " "))
	err = cmd.Run()
	if err != nil {
		return err
	}

	log.Printf("clone finish\n")

	return os.Chdir(m.Repo)
}

func (m *GitManager) fetch() error {
	log.Printf("syncing to %s\n", m.Commit)
	err := exec.Command("git", "branch", "--contains", m.Commit).Run()
	if err == nil {
		return nil
	}

	cmd := exec.Command("git", "fetch", "origin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf(strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "branch", "--contains", m.Commit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf(strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		return err
	}

	// Reset to the merge commit
	cmd = exec.Command("git", "clean", "-f")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf(strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func (m *GitManager) checkout() error {
	cmd := exec.Command("git", "checkout", m.Commit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf(strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func (m *GitManager) AddLabels(issue int, labels ...string) error {
	_, _, err := m.Issues.AddLabelsToIssue(context.TODO(), m.Owner, m.Repo, issue, labels)
	return err
}

func (m *GitManager) AddAssignees(issue int, assignees ...string) error {
	_, _, err := m.Issues.AddAssignees(context.TODO(), m.Owner, m.Repo, issue, assignees)
	return err
}

func (m *GitManager) RemoveLabels(issue int, labels ...string) error {
	for _, l := range labels {
		if _, err := m.Issues.RemoveLabelForIssue(context.TODO(), m.Owner, m.Repo, issue, l); err != nil {
			return err
		}
	}
	return nil
}

func (m *GitManager) RemoveAssignees(issue int, assignees ...string) error {
	_, _, err := m.Issues.RemoveAssignees(context.TODO(), m.Owner, m.Repo, issue, assignees)
	return err
}

func (m *GitManager) commentPrefix(name string) string {
	return fmt.Sprintf("[rollout]: %s", name)
}

func (m *GitManager) GetComment(name string, user string, issue int) (*github.IssueComment, error) {
	comments, _, err := m.Issues.ListComments(context.TODO(), m.Owner, m.Repo, issue, nil)
	if err != nil {
		return nil, err
	}
	expected := m.commentPrefix(name)

	for _, c := range comments {
		madeByUs := c.GetUser() != nil && c.GetUser().GetLogin() == user
		hasPrefix := strings.HasPrefix(c.GetBody(), expected)
		if madeByUs && hasPrefix {
			return c, nil
		}
		fmt.Printf("comment not match %v [%s] [%s]\n", madeByUs, c.GetUser().GetLogin(), user)
		fmt.Printf("comment not match %v [%s] [%s]\n", hasPrefix, c.GetBody(), expected)
	}

	body := m.commentPrefix(name)
	comment := &github.IssueComment{Body: &body}
	comment, _, err = m.Issues.CreateComment(context.TODO(), m.Owner, m.Repo, issue, comment)
	if err != nil {
		return nil, err
	}

	return comment, nil
}

func (m *GitManager) UpdateComment(comment *github.IssueComment, name string, user string, issue int) (
	*github.IssueComment, error) {

	expected := m.commentPrefix(name)
	if !strings.HasPrefix(*comment.Body, expected) {
		body := fmt.Sprintf("%s\n\n%s", expected, *comment.Body)
		comment.Body = &body
	}
	comment, _, err := m.Issues.EditComment(context.TODO(), m.Owner, m.Repo, int(*comment.ID), comment)
	return comment, err
}

func (m *GitManager) UpdateIssueState(issue int, state string) error {
	i := &github.IssueRequest{State: &state}
	_, _, err := m.Issues.Edit(context.TODO(), m.Owner, m.Repo, issue, i)
	return err
}
