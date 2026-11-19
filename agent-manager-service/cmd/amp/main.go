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

// Command amp is the full distribution: the core plus every first-party
// module. Linking a module does not install it; `amp module install` does,
// and the server installs the AI Gateway and the AI Hub at boot unless they
// were installed before or a modules list is declared.
package main

import (
	"github.com/wso2/agent-manager/agent-manager-service/internal/cli"
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/all" // link every module
)

func main() { cli.Main("amp", "ai-gateway", "aihub") }
