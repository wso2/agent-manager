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
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/initfiles"
)

// parseOwner reads "uid:gid" from the environment variable key.
func parseOwner(key string) (*initfiles.Owner, error) {
	o, err := initfiles.ParseOwner(config.Value(key))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return o, nil
}
