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

	"github.com/wso2/agent-manager/agent-manager-service/spec"
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

func TestValidateCreateCustomEvaluatorPayload_CleanSourcePasses(t *testing.T) {
	req := spec.CreateCustomEvaluatorRequest{
		DisplayName: "My Evaluator",
		Type:        "code",
		Level:       "trace",
		Source:      "def evaluate(trace):\n    return {\"score\": 1.0}\n",
	}
	if err := ValidateCreateCustomEvaluatorPayload(req); err != nil {
		t.Errorf("expected clean source to pass, got error: %v", err)
	}
}

func TestValidateCreateCustomEvaluatorPayload_RejectsRiskyImports(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"import os", "import os\ndef evaluate(trace):\n    return {}\n", "import os"},
		{"from os import", "from os import system\n", "import os"},
		{"import subprocess", "import subprocess\n", "import subprocess"},
		{"import socket", "import socket\n", "import socket"},
		{"import ctypes", "import ctypes\n", "import ctypes"},
		{"import importlib", "import importlib\n", "import importlib"},
		{"import urllib", "import urllib.request\n", "import urllib"},
		{"import http", "import http.client\n", "import http"},
		{"import requests", "import requests\n", "import requests"},
		{"import ftplib", "import ftplib\n", "import ftplib"},
		{"import smtplib", "import smtplib\n", "import smtplib"},
		{"dunder import", "x = __import__('os')\n", "__import__"},
		{"os not first in import list", "import sys, os\n", "import os"},
		{"subprocess and socket together", "import subprocess, socket\n", "import subprocess"},
		{"socket last with alias", "import sys, socket as netsock\n", "import socket"},
		{"no space after comma", "import sys,os\n", "import os"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := spec.CreateCustomEvaluatorRequest{
				DisplayName: "Bad Evaluator",
				Type:        "code",
				Level:       "trace",
				Source:      tt.source,
			}
			err := ValidateCreateCustomEvaluatorPayload(req)
			if err == nil {
				t.Fatalf("expected error for source containing %q", tt.name)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestValidateCreateCustomEvaluatorPayload_AvoidsFalsePositives(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"variable named os_path", "os_path = '/tmp'\ndef evaluate(trace):\n    return {}\n"},
		{"comment mentioning subprocess", "# this evaluator does not use subprocess\ndef evaluate(trace):\n    return {}\n"},
		{"docstring mentioning socket", "\"\"\"Not related to the socket module.\"\"\"\ndef evaluate(trace):\n    return {}\n"},
		{"string literal mentioning ctypes", "msg = 'no ctypes here'\ndef evaluate(trace):\n    return {\"score\": 1, \"reason\": msg}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := spec.CreateCustomEvaluatorRequest{
				DisplayName: "Fine Evaluator",
				Type:        "code",
				Level:       "trace",
				Source:      tt.source,
			}
			if err := ValidateCreateCustomEvaluatorPayload(req); err != nil {
				t.Errorf("expected no false positive, got error: %v", err)
			}
		})
	}
}

func TestValidateUpdateCustomEvaluatorPayload_SourceContentChecks(t *testing.T) {
	if err := ValidateUpdateCustomEvaluatorPayload(spec.UpdateCustomEvaluatorRequest{}); err != nil {
		t.Errorf("nil source should not trigger validation: %v", err)
	}

	clean := "def evaluate(trace):\n    return {}\n"
	req := spec.UpdateCustomEvaluatorRequest{Source: &clean}
	if err := ValidateUpdateCustomEvaluatorPayload(req); err != nil {
		t.Errorf("expected clean update source to pass: %v", err)
	}

	risky := "import subprocess\n"
	req2 := spec.UpdateCustomEvaluatorRequest{Source: &risky}
	err := ValidateUpdateCustomEvaluatorPayload(req2)
	if err == nil {
		t.Fatal("expected update with risky source to be rejected")
	}
	if !strings.Contains(err.Error(), "import subprocess") {
		t.Errorf("error %q should mention import subprocess", err)
	}
}
