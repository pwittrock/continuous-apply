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

	"github.com/pwittrock/continuous-apply/pkg/issues"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var v *viper.Viper
var manager *issues.Manager

// manageIssuesCmd represents the applyIssues command
var (
	manageIssuesCmd = &cobra.Command{
		Use:     "manage-issues",
		Short:   "",
		Long:    ``,
		Example: ``,
		PreRunE: validateManageIssues,
		Run:     runManageIssues,
	}
)

func validateManageIssues(cmd *cobra.Command, args []string) error {
	err := v.ReadInConfig() // Find and read the config file
	if err != nil {         // Handle errors reading the config file
		return fmt.Errorf("could not read config file: %s \n", err)
	}

	values := &manager.IssueManagerSpec
	err = v.Unmarshal(values)
	if err != nil { // Handle errors reading the config file
		return fmt.Errorf("could not parse config file: %s \n", err)
	}

	if values.Repo.Owner == "" {
		return fmt.Errorf("must specify repo.owner as the owner of a git repo")
	}

	if values.Repo.Repo == "" {
		return fmt.Errorf("must specify repo.repo as the name of a git repo")
	}

	if values.User == "" {
		return fmt.Errorf("must specify user as the login for a GitHub account")
	}

	if len(values.OpenIssue.Labels) == 0 && values.OpenIssue.State == "" {
		return fmt.Errorf("must specify openIssue.labels or openIssue.state")
	}

	if len(values.OpenActions.AddLabels) == 0 && len(values.OpenActions.AddAssignees) == 0 {
		return fmt.Errorf("must specify openActions.addLabels or openActions.addAssignees")
	}

	if values.Label == "" {
		return fmt.Errorf("must specify label to label managed issues")
	}

	manager.AccessToken = v.GetString("accesstoken")
	if manager.AccessToken == "" {
		return fmt.Errorf("must specify CONTINUOUSAPPLY_ACCESSTOKEN with a GitHub access token")
	}

	return nil
}

func runManageIssues(cmd *cobra.Command, args []string) {
	if err := manager.Run(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	rootCmd.AddCommand(manageIssuesCmd)
	manager = &issues.Manager{}

	v = viper.New()
	v.SetConfigName("config")
	v.AddConfigPath("/etc/continuous-apply/issue-manager")
	v.AddConfigPath("$HOME/.continuous-apply/issue-manager")

	v.SetEnvPrefix("continuousapply")
	v.BindEnv("accesstoken")
}
