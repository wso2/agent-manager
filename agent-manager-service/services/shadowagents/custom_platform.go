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

package shadowagents

import (
	_ "embed" // the Custom platform contract
	"errors"
	"net/url"
	"strings"
)

// CustomPlatformOpenAPI is the OpenAPI document of the agent inventory API a
// customer implements for a Custom platform: GET on the Agents URL, with the
// API key as a bearer token, returning the agents page by page. The console
// offers it for download; the custom connector decodes exactly its fields.
//
//go:embed custom_platform_openapi.yaml
var CustomPlatformOpenAPI string

// Custom platform form fields.
const (
	FieldAgentsURL = "agentsUrl"
	FieldAPIKey    = "apiKey"
)

// maxAgentsURL bounds the Agents URL.
const maxAgentsURL = 2048

// ValidateAgentsURL checks a Custom platform's Agents URL: an absolute http(s)
// URL with a host, and no user info, query string or fragment (the scan adds
// the limit and cursor query parameters).
func ValidateAgentsURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("the Agents URL is required")
	}
	if len(raw) > maxAgentsURL {
		return errors.New("the Agents URL is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("the Agents URL is not a valid URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("the Agents URL must start with https:// or http://")
	}
	if u.User != nil {
		return errors.New("the Agents URL must not contain a user name or password")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("the Agents URL must not contain a query string or a fragment")
	}
	return nil
}
