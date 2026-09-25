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

package utils

import (
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// withFileMountLimits swaps in the given limits for the duration of a test.
func withFileMountLimits(t *testing.T, limits config.FileMountLimitsConfig) {
	t.Helper()
	cfg := config.GetConfig()
	previous := cfg.FileMountLimits
	cfg.FileMountLimits = limits
	t.Cleanup(func() { cfg.FileMountLimits = previous })
}

func fileMount(key, path string, size int) spec.FileMount {
	value := strings.Repeat("a", size)
	return spec.FileMount{Key: key, MountPath: path, Value: &value}
}

func TestValidateFileMounts_DefaultAllowsOneMegabyteFile(t *testing.T) {
	if got := config.GetConfig().FileMountLimits.MaxFileBytes; got != 1000000 {
		t.Fatalf("expected the default per-file cap to be 1000000 bytes, got %d", got)
	}
	if err := ValidateFileMounts([]spec.FileMount{fileMount("app.yaml", "/etc/app/app.yaml", 1000000)}); err != nil {
		t.Fatalf("expected a file at the default cap to pass, got %v", err)
	}
}

func TestValidateFileMounts_UsesConfiguredLimits(t *testing.T) {
	withFileMountLimits(t, config.FileMountLimitsConfig{MaxFileBytes: 100, MaxTotalBytes: 150})

	tests := []struct {
		name        string
		files       []spec.FileMount
		errContains string
	}{
		{
			name:  "file at the configured cap passes",
			files: []spec.FileMount{fileMount("a.txt", "/etc/a.txt", 100)},
		},
		{
			name:        "file over the configured cap rejected",
			files:       []spec.FileMount{fileMount("a.txt", "/etc/a.txt", 101)},
			errContains: "value is 101 bytes; max 100",
		},
		{
			name: "files within their own cap but over the configured total rejected",
			files: []spec.FileMount{
				fileMount("a.txt", "/etc/a.txt", 100),
				fileMount("b.txt", "/etc/b.txt", 51),
			},
			errContains: "total size 151 bytes exceeds limit 150",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFileMounts(tc.files)
			if tc.errContains == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("expected an error containing %q, got %v", tc.errContains, err)
			}
		})
	}
}
