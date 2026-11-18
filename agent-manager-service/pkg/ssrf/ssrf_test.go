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
	"net/netip"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"127.0.0.1", false},
		{"169.254.169.254", false}, // cloud metadata
		{"100.64.0.1", false},      // CGNAT
		{"0.0.0.0", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
	}
	for _, c := range cases {
		ip := netip.MustParseAddr(c.ip)
		if got := IsPublicIP(ip); got != c.want {
			t.Errorf("IsPublicIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestResolvePublicIPs_LiteralsAndLocalhost(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		host    string
		wantErr bool
	}{
		{"8.8.8.8", false},
		{"localhost", true},
		{"foo.localhost", true},
		{"10.0.0.1", true},
		{"169.254.169.254", true},
		{"", true},
	}
	for _, c := range cases {
		_, err := ResolvePublicIPs(ctx, c.host)
		if (err != nil) != c.wantErr {
			t.Errorf("ResolvePublicIPs(%q) err = %v, wantErr %v", c.host, err, c.wantErr)
		}
	}
}

func TestValidateURL(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://8.8.8.8", false},
		{"http://8.8.8.8:8080/path", false},
		{"ftp://8.8.8.8", true},                  // bad scheme
		{"https://user:pass@8.8.8.8", true},      // user info
		{"http://127.0.0.1", true},               // loopback
		{"https://10.0.0.1", true},               // private
		{"https://169.254.169.254/latest", true}, // metadata
		{"not-a-url", true},
		{"https://8.8.8.8:0", true}, // invalid port
	}
	for _, c := range cases {
		err := ValidateURL(ctx, c.url)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateURL(%q) err = %v, wantErr %v", c.url, err, c.wantErr)
		}
	}
}

// Callers collapse these into one message so the reply never reveals internal DNS.
func TestResolutionFailuresWrapErrHostNotPublic(t *testing.T) {
	ctx := context.Background()
	for _, host := range []string{"localhost", "10.0.0.1", "169.254.169.254", "nope.invalid"} {
		if _, err := ResolvePublicIPs(ctx, host); !errors.Is(err, ErrHostNotPublic) {
			t.Errorf("ResolvePublicIPs(%q) err = %v, want ErrHostNotPublic", host, err)
		}
	}
	if err := ValidateURL(ctx, "ftp://8.8.8.8/x"); errors.Is(err, ErrHostNotPublic) {
		t.Errorf("a malformed URL is not a host failure: %v", err)
	}
}
