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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// GeneratedSecrets maps each secret amp init or amp serve generates, when it
// is not set, to its file in SecretsDir. An empty value counts as unset for
// these settings, since an empty secret is never valid.
var GeneratedSecrets = map[string]string{
	"ENCRYPTION_KEY":                   "encryption-key",
	"IDP_CLIENT_SECRET":                "amp-api-client-secret",
	"AMP_CONSOLE_CLIENT_SECRET":        "amp-console-client-secret",
	"THUNDER_CLIENT_SECRET":            "amp-system-client-secret",
	"AMP_ADMIN_PASSWORD":               "admin-password",
	"MODULE_AI_GATEWAY_ADMIN_PASSWORD": "gateway-admin-password",
	"DB_PASSWORD":                      "db-password",
	// The observability store's logins, kept outside the secrets directory.
	"MODULE_OBSERVABILITY_CLICKHOUSE_PASSWORD":          "read-password",
	"MODULE_OBSERVABILITY_CLICKHOUSE_CLEANER_PASSWORD":  "delete-password",
	"MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_PASSWORD": "setup-password",
	"MODULE_OBSERVABILITY_CLICKHOUSE_INGEST_PASSWORD":   "write-password",
}

// GeneratedFile is the file a generated secret is kept in.
func GeneratedFile(key string) (string, bool) {
	name, ok := GeneratedSecrets[key]
	if !ok {
		return "", false
	}
	return filepath.Join(SecretsDir(), name), true
}

// generatedValue reads a generated secret's file, if it exists.
func generatedValue(key string) (string, bool) {
	path, ok := GeneratedFile(key)
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			settingErr(fmt.Errorf("%s: generated secret: %w", key, err))
		}
		return "", false
	}
	return trimNewline(string(data)), true
}

// SetValue is a setting's value as set in the environment or amp.yaml,
// ignoring a generated secret's file.
func SetValue(key string) string {
	v, _ := lookupSet(key)
	return v
}
