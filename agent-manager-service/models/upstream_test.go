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

package models

import "testing"

func strPtr(s string) *string { return &s }

func TestResilienceValidate(t *testing.T) {
	tests := []struct {
		name       string
		resilience *Resilience
		wantErr    bool
	}{
		{name: "nil resilience", resilience: nil, wantErr: false},
		{name: "both fields unset", resilience: &Resilience{}, wantErr: false},
		{name: "valid seconds", resilience: &Resilience{Timeout: strPtr("15s"), IdleTimeout: strPtr("20s")}, wantErr: false},
		{name: "valid milliseconds", resilience: &Resilience{Timeout: strPtr("500ms")}, wantErr: false},
		{name: "valid minutes and hours", resilience: &Resilience{Timeout: strPtr("1m"), IdleTimeout: strPtr("2h")}, wantErr: false},
		{name: "decimal rejected", resilience: &Resilience{Timeout: strPtr("1.5s")}, wantErr: true},
		{name: "zero disables", resilience: &Resilience{Timeout: strPtr("0s")}, wantErr: false},
		{name: "missing unit", resilience: &Resilience{Timeout: strPtr("15")}, wantErr: true},
		{name: "invalid unit", resilience: &Resilience{Timeout: strPtr("15sec")}, wantErr: true},
		{name: "negative", resilience: &Resilience{Timeout: strPtr("-5s")}, wantErr: true},
		{name: "invalid idle timeout", resilience: &Resilience{IdleTimeout: strPtr("bad")}, wantErr: true},
		{name: "empty string", resilience: &Resilience{Timeout: strPtr("")}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resilience.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
