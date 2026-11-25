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
	GatewayURLEnv = "AMCTL_CLOUD_URL"
	// DefaultGatewayURL is production in the US (the prod-v2 environment), the region a cloud
	// login uses when it can't ask. The gateway host, not its api.* alias, is what Agent Manager
	// advertises as its resource.
	DefaultGatewayURL = "https://prod-v2-wso2cloud.gateway.cloud.choreolabs.com"
	// EUGatewayURL is production in the EU (the prod-v2-eu environment).
	EUGatewayURL = "https://prod-v2-eu-wso2cloud.gateway.eu.cloud.choreolabs.com"
	APIPath      = "/agent-manager-service-agent-manager-api"
	// Resource is the RFC 8707 resource indicator for amp:* scopes: Agent Manager's permissions live
	// in the IdP's shared WSO2 Cloud resource server, so it is also the token's audience.
	Resource = "urn:wso2:cloud"
	// legacyResource is the Agent Manager resource server the IdP had before it was folded into
	// Resource; contexts saved back then still store it.
	legacyResource = "urn:wso2:amp"
	// InstanceName is the context cloud logins are saved as; --url logins may not use it.
	InstanceName = "cloud"
)

// BaseDomains are the domains WSO2 Cloud gateways are served under: production and stage on
// cloud.choreolabs.com (and its regional subdomains), dev on cloud.wso2.com.
var BaseDomains = []string{"cloud.choreolabs.com", "cloud.wso2.com"}

// Region is a WSO2 Cloud production region a cloud login can target with --region.
type Region struct {
	Name       string // the --region value
	Label      string
	GatewayURL string
}

// Regions are the production regions, the default first.
var Regions = []Region{
	{Name: "us", Label: "United States", GatewayURL: DefaultGatewayURL},
	{Name: "eu", Label: "Europe", GatewayURL: EUGatewayURL},
}

// APIURL returns the region's Agent Manager API base URL.
func (r Region) APIURL() string { return r.GatewayURL + APIPath }

// RegionByName returns the region whose --region value is name, ignoring case.
func RegionByName(name string) (Region, bool) {
	for _, r := range Regions {
		if strings.EqualFold(r.Name, strings.TrimSpace(name)) {
			return r, true
		}
	}
	return Region{}, false
}

// RegionOfAPIURL returns the region apiURL belongs to, so a stored cloud context keeps its region.
func RegionOfAPIURL(apiURL string) (Region, bool) {
	for _, r := range Regions {
		if strings.TrimRight(apiURL, "/") == r.APIURL() {
			return r, true
		}
	}
	return Region{}, false
}

// RegionNames lists the --region values, for help and error text.
func RegionNames() string {
	names := make([]string, len(Regions))
	for i, r := range Regions {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}

// GatewayOverride returns the AMCTL_CLOUD_URL gateway, or "" when it is unset.
func GatewayOverride(getenv func(string) string) string {
	return strings.TrimSpace(getenv(GatewayURLEnv))
}

// APIURL returns the API base URL for a cloud login: the AMCTL_CLOUD_URL gateway (US production by default) plus APIPath.
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
	if !isCloudHost(strings.ToLower(u.Hostname())) {
		return false
	}
	path := strings.TrimRight(u.Path, "/")
	return path == APIPath || strings.HasPrefix(path, APIPath+"/")
}

// UpdateResource replaces the retired Agent Manager resource indicator a context may have been saved
// with by the current one, so signing in again to an old cloud context still gets amp:* scopes.
func UpdateResource(resource string) string {
	if resource == legacyResource {
		return Resource
	}
	return resource
}

// isCloudHost reports whether host is one of BaseDomains or a subdomain of one.
func isCloudHost(host string) bool {
	for _, d := range BaseDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// LoginHint tells a user who passed cloud URL raw to --url to use --cloud; releasesURL is where to get an amctl that has it.
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
