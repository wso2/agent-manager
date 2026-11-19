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

// Package reqorigin works out the origin a caller used to reach this server.
package reqorigin

import (
	"net/http"
	"regexp"
	"strings"
)

// Of is the origin the caller used, for a server that does not know its
// public URL. A TLS-terminating proxy or ingress reports the original scheme
// and host in X-Forwarded-Proto and X-Forwarded-Host; the first value of each
// is used when it is valid, and ignored otherwise. Use it only for answers
// meant for this caller, never for anything cached or shared.
func Of(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := firstForwarded(r.Header.Get("X-Forwarded-Proto")); p == "http" || p == "https" {
		scheme = p
	}
	host := r.Host
	if h := firstForwarded(r.Header.Get("X-Forwarded-Host")); h != "" && validHost.MatchString(h) {
		host = h
	}
	return scheme + "://" + host
}

// validHost is a host[:port]: letters, digits, dots, dashes, and an IPv6
// literal's brackets and colons.
var validHost = regexp.MustCompile(`^[A-Za-z0-9.\-\[\]:]+$`)

// firstForwarded is the first value of a comma-separated forwarded header
// (the one the outermost proxy set), lower-cased for the scheme check.
func firstForwarded(v string) string {
	first, _, _ := strings.Cut(v, ",")
	return strings.ToLower(strings.TrimSpace(first))
}
