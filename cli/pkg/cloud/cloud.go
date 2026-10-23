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

// Package cloud holds what amctl needs to reach Agent Manager on WSO2 Cloud, so that
// cloud-only knowledge stays out of the instance-agnostic login and API code.
package cloud

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	GatewayURLEnv     = "AMCTL_CLOUD_URL"
	DefaultGatewayURL = "https://production-wso2cloud.gateway.cloud.wso2.com"
	APIPath           = "/agent-manager-service-agent-manager-api"
	Resource          = "urn:wso2:amp"
	BaseDomain        = "cloud.wso2.com"
)

// APIURL returns the API base URL for --cloud: the AMCTL_CLOUD_URL gateway (production by default) plus APIPath.
func APIURL(getenv func(string) string) string {
	gateway := strings.TrimSpace(getenv(GatewayURLEnv))
	if gateway == "" {
		gateway = DefaultGatewayURL
	}
	return strings.TrimRight(gateway, "/") + APIPath
}

// IsAPIURL reports whether raw is an Agent Manager API URL on a WSO2 Cloud gateway.
func IsAPIURL(raw string) bool {
	u, err := parseURL(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != BaseDomain && !strings.HasSuffix(host, "."+BaseDomain) {
		return false
	}
	path := strings.TrimRight(u.Path, "/")
	return path == APIPath || strings.HasPrefix(path, APIPath+"/")
}

// tells a user who passed cloud URL raw to --url to use --cloud; releasesURL is where to get an amctl that has it.
func LoginHint(raw, releasesURL string) string {
	return fmt.Sprintf("%s is a WSO2 Cloud URL; log in with --cloud instead: export %s=%s && amctl login --cloud (if --cloud is unknown, download the latest amctl from %s)",
		raw, GatewayURLEnv, gatewayOf(raw), releasesURL)
}

// gatewayOf returns the scheme and host of raw, or "" if it does not parse.
func gatewayOf(raw string) string {
	u, err := parseURL(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// parseURL parses raw, assuming https:// when it has no scheme so a bare host/path
// still yields a hostname.
func parseURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return url.Parse(raw)
}
