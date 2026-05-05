// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package cmdutil

import (
	"github.com/spf13/cobra"
)

// EnableProjectOverride registers the persistent --project flag and its
// dynamic shell completion. Use on the agent command (or any command that
// scopes operations to a project).
func EnableProjectOverride(cmd *cobra.Command, f *Factory) {
	cmd.PersistentFlags().String("project", "", "Project to operate on (required for project-scoped commands)")
	_ = cmd.RegisterFlagCompletionFunc("project", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return CompleteProjects(cmd, f), cobra.ShellCompDirectiveNoFileComp
	})
}
