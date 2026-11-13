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

package services

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// One Thunder ships per installation. The default environment's agent
// identities live in the platform Thunder (THUNDER_BASE_URL), reached with the
// platform system client; no other environment has a Thunder.

// platformThunderURL returns the platform Thunder's URL for envName, or ""
// when envName is not the default environment or no platform Thunder is configured.
func platformThunderURL(envName string) string {
	if envName != models.DefaultEnvironmentName {
		return ""
	}
	return config.GetConfig().Thunder.BaseURL
}

// NewPlatformThunderSecretReader reads the platform system client for the
// default environment, and ErrThunderNotProvisioned for any other.
func NewPlatformThunderSecretReader() thundersvc.ReadSystemClientFunc {
	return func(_ context.Context, _, envName string) (string, string, error) {
		cfg := config.GetConfig().Thunder
		if platformThunderURL(envName) == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
			return "", "", thundersvc.ErrThunderNotProvisioned
		}
		return cfg.ClientID, cfg.ClientSecret, nil
	}
}

// NewPlatformThunderURLReader reads the platform Thunder's URL for the default
// environment, marked caller-supplied so the resolver probes it SSRF-hardened.
func NewPlatformThunderURLReader() thundersvc.ReadThunderURLFunc {
	return func(_ context.Context, _, envName string) (string, bool, error) {
		return platformThunderURL(envName), true, nil
	}
}
