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
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// One Thunder ships per installation. When the default environment has no
// Thunder registration of its own, its agent identities live in the
// platform Thunder (THUNDER_BASE_URL), reached with the platform system
// client. A registration, when present, always wins.

// platformThunderURL returns the platform Thunder's URL for envName, or ""
// when envName is not the default environment or no platform Thunder is configured.
func platformThunderURL(envName string) string {
	if envName != models.DefaultEnvironmentName {
		return ""
	}
	return config.GetConfig().Thunder.BaseURL
}

// platformThunderSystemClient returns the platform system client for envName,
// with ok false when the platform Thunder cannot stand in for it.
func platformThunderSystemClient(envName string) (clientID, clientSecret string, ok bool) {
	if platformThunderURL(envName) == "" {
		return "", "", false
	}
	cfg := config.GetConfig().Thunder
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return "", "", false
	}
	return cfg.ClientID, cfg.ClientSecret, true
}

// withPlatformThunder fills an unregistered default environment's record with
// the platform Thunder's URL, as a caller-supplied URL (no handle).
func withPlatformThunder(rec models.ThunderURLRecord, envName string) models.ThunderURLRecord {
	if rec.URL != "" {
		return rec
	}
	if url := platformThunderURL(envName); url != "" {
		return models.ThunderURLRecord{URL: url}
	}
	return rec
}
