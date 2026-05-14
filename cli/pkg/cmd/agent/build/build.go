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

package build

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// resolveAgentWithRemaining handles commands that accept [agent] <build-name>.
// When a single arg is provided, it tries linked-context first; if that fails
// it treats the arg as the agent name.
func resolveAgentWithRemaining(resolve func([]string) (string, []string, error), args []string) (string, []string, error) {
	if len(args) == 1 {
		agent, _, err := resolve(nil)
		if err == nil {
			return agent, args, nil
		}
		return resolve(args)
	}
	return resolve(args)
}

func NewBuildCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Manage agent builds",
	}
	cmd.AddCommand(NewCreateCmd(f))
	cmd.AddCommand(NewListCmd(f))
	cmd.AddCommand(NewGetCmd(f))
	cmd.AddCommand(NewLogsCmd(f))
	return cmd
}
