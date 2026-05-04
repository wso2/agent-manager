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

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/amctl/cmd/agent"
	amcontext "github.com/wso2/agent-manager/internal/amctl/cmd/context"
	"github.com/wso2/agent-manager/internal/amctl/cmd/project"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/version"
)

func NewRootCmd(f *cmdutil.Factory) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "amctl",
		Short:         "Interact with Agent Manager via CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.Version = version.Short()
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return cmdutil.FlagErrorWrap(err)
	})
	cmdutil.EnableOrgOverride(cmd, f)
	cmd.PersistentFlags().BoolVar(&f.IOStreams.JSON, "json", false, "Output as JSON envelopes")

	cmd.AddCommand(NewLoginCmd(f))
	cmd.AddCommand(agent.NewAgentCmd(f))
	cmd.AddCommand(amcontext.NewContextCmd(f))
	cmd.AddCommand(project.NewProjectCmd(f))
	cmd.AddCommand(NewVersionCmd())

	return cmd, nil
}
