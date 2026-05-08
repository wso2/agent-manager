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

package framework

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"
)

// LoadTestData reads a JSON file from the testdata directory and unmarshals it into dest.
// testDataDir is the absolute or relative path to the testdata directory.
func LoadTestData(testDataDir, relPath string, dest any) {
	data, err := os.ReadFile(filepath.Join(testDataDir, relPath))
	Expect(err).NotTo(HaveOccurred(), "failed to read testdata file: %s", relPath)
	Expect(json.Unmarshal(data, dest)).To(Succeed(), "failed to unmarshal testdata file: %s", relPath)
}

// InjectEnvVars populates environment variable values in Configurations from the provided map.
func InjectEnvVars(cfg *Configurations, envVars map[string]string) {
	Expect(cfg).NotTo(BeNil(), "agent configurations must be set")
	for i := range cfg.Env {
		val, ok := envVars[cfg.Env[i].Key]
		if !ok || val == "" {
			continue
		}
		cfg.Env[i].Value = val
	}
}
