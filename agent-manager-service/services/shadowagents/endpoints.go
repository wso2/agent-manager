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
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var placeholderRE = regexp.MustCompile(`\{([a-zA-Z]+)\}`)

// ValidateEndpoint checks an endpoint the user entered: an absolute http(s)
// URL with a host, no user info, query or fragment, and only the
// placeholders the endpoint declares.
func ValidateEndpoint(spec EndpointSpec, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%s: a URL is required", spec.Label)
	}
	allowed := map[string]bool{}
	for _, p := range spec.Placeholders {
		allowed[p] = true
	}
	for _, m := range placeholderRE.FindAllStringSubmatch(raw, -1) {
		if !allowed[m[1]] {
			return fmt.Errorf("%s: unknown placeholder {%s}", spec.Label, m[1])
		}
	}
	// Parse with the placeholders filled in, so {location}-host parses.
	probe := placeholderRE.ReplaceAllString(raw, "x")
	u, err := url.Parse(probe)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s: not a valid URL", spec.Label)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("%s: the URL must start with https:// or http://", spec.Label)
	}
	if u.User != nil {
		return fmt.Errorf("%s: the URL must not contain a user name or password", spec.Label)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s: the URL must not contain a query or fragment", spec.Label)
	}
	return nil
}

// IsOfficialHost reports whether an endpoint's host is one the provider
// officially serves it from. A false result only produces a warning.
func IsOfficialHost(spec EndpointSpec, raw string) bool {
	if len(spec.OfficialHostSuffixes) == 0 {
		return true
	}
	u, err := url.Parse(placeholderRE.ReplaceAllString(strings.TrimSpace(raw), "x"))
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range spec.OfficialHostSuffixes {
		s = strings.ToLower(s)
		if host == strings.TrimPrefix(s, ".") || strings.HasSuffix(host, s) || strings.HasSuffix(host, "."+s) || strings.HasSuffix(host, "-"+s) {
			return true
		}
	}
	return false
}

// Expand fills an endpoint template's placeholders and trims a trailing
// slash. It fails if a placeholder is left unfilled.
func Expand(tmpl string, vars map[string]string) (string, error) {
	var missing []string
	out := placeholderRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := m[1 : len(m)-1]
		v, ok := vars[name]
		if !ok || v == "" {
			missing = append(missing, name)
			return m
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("the endpoint needs a value for {%s}", strings.Join(missing, "}, {"))
	}
	return strings.TrimRight(strings.TrimSpace(out), "/"), nil
}

// EndpointOf returns the connection's value for an endpoint key, falling
// back to the provider's official default.
func EndpointOf(conn Connection, key string) string {
	if v := strings.TrimSpace(conn.Config.Endpoints[key]); v != "" {
		return v
	}
	return DefaultEndpoints(conn.Provider)[key]
}

// Per-value endpoints. An endpoint with one placeholder ({region},
// {location}) is used once per value the connection scans; the user can
// replace the URL of one value, stored as "<endpoint key>@<value>".
const valueSep = "@"

// placeValueRE bounds a placeholder value (a region or location code).
var placeValueRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// awsRegionRE matches an AWS region code such as us-east-1 or us-gov-west-1.
var awsRegionRE = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)

// ValueOverrideKey is the endpoints-map key of one value's URL.
func ValueOverrideKey(key, value string) string { return key + valueSep + value }

// SplitValueKey splits an endpoints-map key "<key>@<value>"; ok is false for
// a plain endpoint key.
func SplitValueKey(k string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(k, valueSep)
	return key, value, ok && key != "" && value != ""
}

// ValidRegion reports whether s is an AWS region code.
func ValidRegion(s string) bool { return awsRegionRE.MatchString(s) }

// ValidPlaceValue reports whether s can fill an endpoint placeholder.
func ValidPlaceValue(s string) bool { return placeValueRE.MatchString(s) }

// PerValue reports whether the endpoint is used once per value.
func (e EndpointSpec) PerValue() bool {
	return len(e.Placeholders) == 1 && (e.FillFrom != "" || len(e.FillValues) > 0)
}

// ValueEndpoint returns an endpoint's URL for one value of its placeholder:
// the URL the user set for that value, or the template filled in.
func ValueEndpoint(conn Connection, key, placeholder, value string) (string, error) {
	if v := strings.TrimSpace(conn.Config.Endpoints[ValueOverrideKey(key, value)]); v != "" {
		return strings.TrimRight(v, "/"), nil
	}
	return Expand(EndpointOf(conn, key), map[string]string{placeholder: value})
}

// RegionEndpoint is ValueEndpoint for a {region} endpoint.
func RegionEndpoint(conn Connection, key, region string) (string, error) {
	return ValueEndpoint(conn, key, "region", region)
}

// MustExpand is Expand for templates without placeholders, or with every
// placeholder given; it returns "" when the template cannot be expanded.
func MustExpand(tmpl string, vars map[string]string) string {
	s, err := Expand(tmpl, vars)
	if err != nil {
		return ""
	}
	return s
}
