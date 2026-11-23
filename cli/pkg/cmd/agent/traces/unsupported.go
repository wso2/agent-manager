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

package traces

import (
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// moduleUnsupportedFlags are the flags the observability module can't serve
// yet: it refuses their query params, or ignores include (the --show-* columns).
var moduleUnsupportedFlags = []string{
	"status", "min-duration", "min-tokens", "min-spans", "model", "conversation", "no-conversation",
	"tool", "tool-error", "mcp-server", "cursor", "show-models", "show-tools", "show-mcp-servers",
	"max-latency", "max-tokens", "max-spans",
}

// moduleUnsupportedConditions are the --condition values that set a refused filter.
var moduleUnsupportedConditions = []string{
	conditionErrorStatus, conditionHighLatency, conditionHighTokenUsage, conditionExcessiveSteps,
}

// hideUnsupportedFlags hides cmd's flags the observability module can't serve yet.
func hideUnsupportedFlags(cmd *cobra.Command) {
	for _, name := range moduleUnsupportedFlags {
		if cmd.Flags().Lookup(name) != nil {
			_ = cmd.Flags().MarkHidden(name)
		}
	}
}

// checkModuleSupport refuses a flag or --condition the observability module can't serve yet.
func checkModuleSupport(fs *pflag.FlagSet, condition string) error {
	for _, name := range moduleUnsupportedFlags {
		if fs.Changed(name) {
			return cmdutil.FlagErrorf("--%s is not supported by the observability module yet", name)
		}
	}
	if slices.Contains(moduleUnsupportedConditions, condition) {
		return cmdutil.FlagErrorf("--condition %s is not supported by the observability module yet", condition)
	}
	return nil
}
