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
)

func TestValidateTemplateHandle(t *testing.T) {
	tests := []struct {
		name    string
		handle  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid handle with alphanumeric",
			handle:  "openai",
			wantErr: false,
		},
		{
			name:    "valid handle with hyphens",
			handle:  "azure-openai",
			wantErr: false,
		},
		{
			name:    "valid handle with underscores",
			handle:  "aws_bedrock",
			wantErr: false,
		},
		{
			name:    "valid handle with mixed characters",
			handle:  "my-template_v1",
			wantErr: false,
		},
		{
			name:    "valid handle with numbers",
			handle:  "template123",
			wantErr: false,
		},
		{
			name:    "empty handle",
			handle:  "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "handle too long",
			handle:  strings.Repeat("a", 256),
			wantErr: true,
			errMsg:  "must not exceed 255 characters",
		},
		{
			name:    "handle with spaces",
			handle:  "my template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with special characters",
			handle:  "template@123",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with dots",
			handle:  "my.template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with forward slash",
			handle:  "my/template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with backslash",
			handle:  "my\\template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with null byte",
			handle:  "template\x00",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle at max length",
			handle:  strings.Repeat("a", 255),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateHandle(tt.handle)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateTemplateHandle() expected error but got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateTemplateHandle() error = %v, want error containing %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateTemplateHandle() unexpected error = %v", err)
				}
			}
		})
	}
}
