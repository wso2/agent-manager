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

package card

import (
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func Test_NewCardCmd_registersSubcommands(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCardCmd(&cmdutil.Factory{IOStreams: ios})

	if cmd.Use != "card" {
		t.Errorf("Use = %q, want card", cmd.Use)
	}
	want := map[string]bool{"set": false, "refresh": false, "get": false}
	for _, c := range cmd.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("subcommand %q not registered under card", name)
		}
	}
}
