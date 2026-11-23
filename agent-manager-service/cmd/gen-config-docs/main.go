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
// Command gen-config-docs writes the configuration reference: run as
// "make config-docs".
package main

import (
	"fmt"
	"os"

	_ "github.com/wso2/agent-manager/agent-manager-service/ampinit" // amp init's settings
	"github.com/wso2/agent-manager/agent-manager-service/internal/configdocs"
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/all"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen-config-docs <output.md>")
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[1], []byte(configdocs.Render()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
