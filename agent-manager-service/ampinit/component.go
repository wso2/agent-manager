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

package ampinit

import (
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// Components returns the names "amp init" takes: the core's (ThunderID and
// file-based sign-in) first, then every compiled-in module that implements
// modkit.InitComponent, by module name.
func Components() []string {
	names := []string{"thunder", "fileauth"}
	for _, m := range modkit.Registered() {
		if _, ok := m.(modkit.InitComponent); ok {
			names = append(names, m.Manifest().Name)
		}
	}
	return names
}

// initComponent returns the compiled-in module named name if it writes files
// at "amp init".
func initComponent(name string) (modkit.InitComponent, bool) {
	for _, m := range modkit.Registered() {
		if c, ok := m.(modkit.InitComponent); ok && m.Manifest().Name == name {
			return c, true
		}
	}
	return nil, false
}
