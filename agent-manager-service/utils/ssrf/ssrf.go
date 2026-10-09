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

// Package ssrf provides SSRF (Server-Side Request Forgery) protection for
// outbound HTTP requests to user-supplied URLs. It validates that a URL's host
// resolves only to public IP addresses, and builds an *http.Client that pins
// connections to those validated IPs — re-resolving at dial time to defeat DNS
// rebinding and re-validating every redirect hop.
package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// blockedIPPrefixes are IP ranges that must never be reached by SSRF-guarded
// requests: loopback, private (RFC 1918), link-local (cloud metadata), CGNAT,
// documentation, multicast and their IPv6 equivalents.
var blockedIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// ValidateURL validates that rawURL is a well-formed http(s) URL with no user
// info and a host that resolves only to public IP addresses.
func ValidateURL(ctx context.Context, rawURL string) error {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return err
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("url must use http or https")
	}
	if parsedURL.User != nil {
		return fmt.Errorf("url must not include user information")
	}
	if parsedURL.Host == "" || parsedURL.Hostname() == "" {
		return fmt.Errorf("url host is required")
	}
	if parsedURL.Port() != "" {
		port, err := strconv.Atoi(parsedURL.Port())
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("url port is invalid")
		}
	}
	if err := ValidateHost(ctx, parsedURL.Hostname()); err != nil {
		return err
	}
	return nil
}

// ErrHostNotPublic marks a host that is unresolvable, localhost or non-public; its detail can reveal internal DNS.
var ErrHostNotPublic = errors.New("url host is not publicly reachable")

type hostNotPublicError struct{ err error }

func (e *hostNotPublicError) Error() string   { return e.err.Error() }
func (e *hostNotPublicError) Unwrap() []error { return []error{e.err, ErrHostNotPublic} }

func hostNotPublic(format string, a ...any) error {
	return &hostNotPublicError{err: fmt.Errorf(format, a...)}
}

// ValidateHost validates that host resolves only to public IP addresses.
func ValidateHost(ctx context.Context, host string) error {
	_, err := ResolvePublicIPs(ctx, host)
	return err
}

// ResolvePublicIPs resolves host and returns its IP addresses, failing if the
// host is localhost or any resolved address is non-public.
func ResolvePublicIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil, fmt.Errorf("url host is required")
	}
	if strings.Contains(host, "%") {
		return nil, fmt.Errorf("url host must not include an IPv6 zone identifier")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, hostNotPublic("url host must not resolve to localhost")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is4In6() {
			ip = ip.Unmap()
		}
		if !IsPublicIP(ip) {
			return nil, hostNotPublic("url host resolves to a non-public IP address")
		}
		return []netip.Addr{ip}, nil
	}

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, hostNotPublic("url host could not be resolved: %w", err)
	}
	if len(ips) == 0 {
		return nil, hostNotPublic("url host could not be resolved")
	}
	publicIPs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if ip.Is4In6() {
			ip = ip.Unmap()
		}
		if !IsPublicIP(ip) {
			return nil, hostNotPublic("url host resolves to a non-public IP address")
		}
		publicIPs = append(publicIPs, ip)
	}
	return publicIPs, nil
}

// IsPublicIP reports whether ip is a globally routable public unicast address
// that is not in the blocked-prefix list.
func IsPublicIP(ip netip.Addr) bool {
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blockedIPPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

// NewClient returns an *http.Client hardened against SSRF. Connections are
// pinned to validated public IPs (re-resolved at dial time to defeat DNS
// rebinding) and every redirect hop is re-validated. timeout bounds both the
// dial and the overall request.
func NewClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	d := &dialer{dialer: &net.Dialer{Timeout: timeout}}
	transport.DialContext = d.DialContext

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if err := ValidateURL(req.Context(), req.URL.String()); err != nil {
				return fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
			}
			return nil
		},
	}
}

type dialer struct {
	dialer *net.Dialer
}

func (d *dialer) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := ResolvePublicIPs(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}

	var firstErr error
	for _, ip := range ips {
		if network == "tcp4" && !ip.Is4() {
			continue
		}
		if network == "tcp6" && !ip.Is6() {
			continue
		}
		conn, err := d.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("url host has no public IP addresses for network %s", network)
}
