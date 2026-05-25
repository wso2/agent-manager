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

package instrumentation

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed baseline.json
var baselineJSON []byte

// decodeBaseline parses the embedded baseline.json into Version entries.
// Source defaults to "bundled".
func decodeBaseline() ([]Version, error) {
	var raw []Version
	if err := json.Unmarshal(baselineJSON, &raw); err != nil {
		return nil, fmt.Errorf("decode embedded instrumentation baseline: %w", err)
	}
	for i := range raw {
		raw[i].Source = SourceBundled
	}
	return raw, nil
}
