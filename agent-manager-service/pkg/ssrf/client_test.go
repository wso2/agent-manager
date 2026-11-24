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
package ssrf

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The guarded client must refuse to connect to a loopback server, so the handler never runs.
func TestNewClientRefusesNonPublicTargets(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	client := NewClient(2 * time.Second)
	if client.Timeout != 2*time.Second {
		t.Fatalf("client timeout = %v, want 2s", client.Timeout)
	}
	resp, err := client.Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request to a loopback server succeeded")
	}
	if !errors.Is(err, ErrInvalidURL) || !errors.Is(err, ErrHostNotPublic) {
		t.Fatalf("err = %v, want ErrInvalidURL and ErrHostNotPublic", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("loopback server was reached %d times", hits.Load())
	}
}

func TestNewClientRevalidatesRedirects(t *testing.T) {
	client := NewClient(time.Second)
	req := httptest.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data", nil)

	err := client.CheckRedirect(req, []*http.Request{{}})
	if !errors.Is(err, ErrInvalidURL) || !errors.Is(err, ErrHostNotPublic) {
		t.Fatalf("redirect to metadata IP: err = %v, want ErrInvalidURL wrapping ErrHostNotPublic", err)
	}

	public := httptest.NewRequest(http.MethodGet, "https://8.8.8.8/", nil)
	if err := client.CheckRedirect(public, []*http.Request{{}}); err != nil {
		t.Fatalf("redirect to a public IP: %v", err)
	}

	via := make([]*http.Request, 10)
	if err := client.CheckRedirect(public, via); err == nil || !strings.Contains(err.Error(), "10 redirects") {
		t.Fatalf("eleventh hop: err = %v, want redirect limit", err)
	}
}

func TestDialContext(t *testing.T) {
	d := &dialer{dialer: nil}

	if _, err := d.DialContext(context.Background(), "tcp", "no-port"); err == nil {
		t.Fatal("address without a port was accepted")
	}

	_, err := d.DialContext(context.Background(), "tcp", "10.1.2.3:80")
	if !errors.Is(err, ErrInvalidURL) || !errors.Is(err, ErrHostNotPublic) {
		t.Fatalf("private address: err = %v, want ErrInvalidURL wrapping ErrHostNotPublic", err)
	}

	_, err = d.DialContext(context.Background(), "tcp6", "8.8.8.8:443")
	if err == nil || !strings.Contains(err.Error(), "no public IP addresses for network tcp6") {
		t.Fatalf("IPv4 host on tcp6: err = %v", err)
	}
	_, err = d.DialContext(context.Background(), "tcp4", "[2606:4700:4700::1111]:443")
	if err == nil || !strings.Contains(err.Error(), "no public IP addresses for network tcp4") {
		t.Fatalf("IPv6 host on tcp4: err = %v", err)
	}
}

// A dial that fails returns the dialer's error rather than a generic one.
func TestDialContextReturnsDialError(t *testing.T) {
	d := NewClient(time.Second).Transport.(*http.Transport).DialContext
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d(ctx, "tcp4", "8.8.8.8:443")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestResolvePublicIPsEdgeCases(t *testing.T) {
	ctx := context.Background()

	if _, err := ResolvePublicIPs(ctx, "fe80::1%en0"); err == nil || !strings.Contains(err.Error(), "zone identifier") {
		t.Fatalf("zone identifier: err = %v", err)
	}
	if _, err := ResolvePublicIPs(ctx, "LOCALHOST."); !errors.Is(err, ErrHostNotPublic) {
		t.Fatalf("trailing-dot uppercase localhost: err = %v", err)
	}
	if _, err := ResolvePublicIPs(ctx, "::ffff:10.0.0.1"); !errors.Is(err, ErrHostNotPublic) {
		t.Fatalf("IPv4-mapped private address: err = %v", err)
	}
	ips, err := ResolvePublicIPs(ctx, "::ffff:8.8.8.8")
	if err != nil {
		t.Fatalf("IPv4-mapped public address: %v", err)
	}
	if want := netip.MustParseAddr("8.8.8.8"); len(ips) != 1 || ips[0] != want {
		t.Fatalf("ips = %v, want [%v] (unmapped)", ips, want)
	}
	// The detail stays available to logs while the sentinel classifies it.
	_, err = ResolvePublicIPs(ctx, "10.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "non-public IP") {
		t.Fatalf("err = %v, want non-public detail", err)
	}
}

func TestValidateURLShape(t *testing.T) {
	u, err := ValidateURLShape("https://example.com:8443/p?q=1")
	if err != nil {
		t.Fatalf("ValidateURLShape: %v", err)
	}
	if u.Hostname() != "example.com" || u.Port() != "8443" {
		t.Fatalf("parsed host %q port %q", u.Hostname(), u.Port())
	}
	for _, bad := range []string{"https:///path", "https://example.com:70000", "https://example.com:abc"} {
		if _, err := ValidateURLShape(bad); err == nil {
			t.Errorf("ValidateURLShape(%q) accepted", bad)
		}
	}
	if !IsPublicIP(netip.MustParseAddr("::ffff:8.8.8.8")) {
		t.Error("IPv4-mapped public address reported non-public")
	}
	if IsPublicIP(netip.Addr{}) {
		t.Error("zero address reported public")
	}
}
