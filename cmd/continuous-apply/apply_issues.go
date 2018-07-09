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

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/pwittrock/continuous-apply/pkg/git"
	"github.com/pwittrock/continuous-apply/pkg/poller"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// applyIssuesCmd represents the applyIssues command
var (
	p              = poller.Poller{}
	applyIssuesCmd = &cobra.Command{
		Use:     "apply-issues",
		Short:   "",
		Long:    ``,
		Example: ``,
		PreRunE: validate,
		Run:     run,
	}
)

func validate(cmd *cobra.Command, args []string) error {
	if p.Name == "" {
		return fmt.Errorf("--name cannot be empty")
	}
	if p.Owner == "" {
		return fmt.Errorf("--owner cannot be empty")
	}
	if p.Repo == "" {
		return fmt.Errorf("--repo cannot be empty")
	}
	return nil
}

func run(cmd *cobra.Command, args []string) {
	var err error
	if p.GitClient, err = git.NewManager(p.Owner, p.Repo, ""); err != nil {
		log.Fatal(err)
	}
	if err = p.Run(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	p = poller.Poller{}
	rootCmd.AddCommand(applyIssuesCmd)

	applyIssuesCmd.Flags().StringVar(&p.Owner, "owner", "", "GitHub user or org")
	applyIssuesCmd.MarkFlagRequired("owner")

	applyIssuesCmd.Flags().StringVar(&p.Repo, "repo", "", "GitHub repo")
	applyIssuesCmd.MarkFlagRequired("repo")

	// Status Update flags
	applyIssuesCmd.Flags().StringVar(&p.Name, "name", "", "Name of the rollout")
	applyIssuesCmd.MarkFlagRequired("name")

	applyIssuesCmd.Flags().StringVar(&p.User, "user", "", "GitHub we are acting as")
	applyIssuesCmd.MarkFlagRequired("user")

	applyIssuesCmd.Flags().StringSliceVar(&p.ApplyTargets, "apply-targets",
		[]string{"./"}, "")

	applyIssuesCmd.Flags().StringVar(&p.Type, "sync-type", "issue", "issue || pr")
	applyIssuesCmd.Flags().StringVar(&p.RolloutType, "rollout-type", "sequential", "sequential || parallel")

	applyIssuesCmd.Flags().StringSliceVar(&p.MatchLabels, "match-labels", []string{},
		"Only apply issues with these labels.")
	applyIssuesCmd.Flags().StringVar(&p.MatchAssignee, "match-assignee", "", "")
	applyIssuesCmd.Flags().StringVar(&p.MatchState, "match-state", "", "")
	applyIssuesCmd.Flags().StringVar(&p.MatchMilestone, "match-milestone", "", "")

	applyIssuesCmd.Flags().StringSliceVar(&p.BeforeAddLabels, "before-add-labels", []string{},
		"Labels to set before starting a rollout.")
	applyIssuesCmd.Flags().StringSliceVar(&p.BeforeRemoveLabels, "before-remove-labels", []string{},
		"Labels to remove before starting a rollout.")
	applyIssuesCmd.Flags().StringSliceVar(&p.BeforeAddAssignees, "before-add-assignees", []string{},
		"Assignees to set after completing a rollout.")
	applyIssuesCmd.Flags().StringSliceVar(&p.BeforeRemoveAssignees, "before-remove-assignees", []string{},
		"Assignees to remove after completing a rollout.")
	applyIssuesCmd.Flags().StringVar(&p.BeforeSetState, "before-set-state", "", "")

	applyIssuesCmd.Flags().StringSliceVar(&p.AfterAddLabels, "after-add-labels", []string{},
		"Labels to set before starting a rollout.")
	applyIssuesCmd.Flags().StringSliceVar(&p.AfterRemoveLabels, "after-remove-labels", []string{},
		"Labels to remove before starting a rollout.")
	applyIssuesCmd.Flags().StringSliceVar(&p.AfterAddAssignees, "after-add-assignees", []string{},
		"Assignees to set after completing a rollout.")
	applyIssuesCmd.Flags().StringSliceVar(&p.AfterRemoveAssignees, "after-remove-assignees", []string{},
		"Assignees to remove after completing a rollout.")
	applyIssuesCmd.Flags().StringVar(&p.AfterSetState, "after-set-state", "", "")

	applyIssuesCmd.Flags().StringSliceVar(&p.SkipAddLabels, "skip-add-labels", []string{}, "")
	applyIssuesCmd.Flags().StringVar(&p.SkipSetState, "skip-set-state", "", "")

	applyIssuesCmd.Flags().DurationVar(&p.Pause, "pause", 1*time.Second,
		"Pause time between checking rollout status")

}
