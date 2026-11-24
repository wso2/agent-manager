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
package config

import (
	"path/filepath"
	"strings"
)

// DefaultDataDir is where state lives when AMP_DATA_DIR is unset: the
// SQLite database, certificates and generated secrets.
const DefaultDataDir = "data"

// DataDir is AMP_DATA_DIR, else DefaultDataDir.
func DataDir() string {
	if v := strings.TrimSpace(Value("AMP_DATA_DIR")); v != "" {
		return v
	}
	return DefaultDataDir
}

// SecretsDir is AMP_SECRETS_DIR, else the secrets directory under DataDir.
func SecretsDir() string {
	if v := strings.TrimSpace(Value("AMP_SECRETS_DIR")); v != "" {
		return v
	}
	return filepath.Join(DataDir(), "secrets")
}
