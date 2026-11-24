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

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/cli/pkg/auth"
	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cloud"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/version"
)

// orgsClient serves /orgs with the named organizations, or a status when != 200.
func orgsClient(t *testing.T, status int, names ...string) func(context.Context) (*amsvc.ClientWithResponses, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"code":"BOOM","message":"orgs unavailable"}`))
			return
		}
		items := []map[string]any{}
		for _, n := range names {
			items = append(items, map[string]any{"name": n, "createdAt": "2026-01-01T00:00:00Z"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"organizations": items, "limit": 2, "offset": 0, "total": len(items)})
	}))
	t.Cleanup(srv.Close)
	c, err := amsvc.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return c, nil }
}

func jsonHarness(t *testing.T, instances map[string]config.Instance, current string) (*loginHarness, *bytes.Buffer) {
	t.Helper()
	h := newLoginHarness(t, instances, current)
	h.opts.IO.JSON = true
	out := &bytes.Buffer{}
	h.opts.IO.Out = out
	return h, out
}

func errorCode(t *testing.T, raw string) string {
	t.Helper()
	var env struct {
		Error clierr.CLIError `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("not JSON: %v: %q", err, raw)
	}
	return env.Error.Code
}

func TestLogin_FlagValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(o *LoginOptions)
		wantMsg string
	}{
		{"client secret needs client id", func(o *LoginOptions) { o.ClientSecret = "s"; o.URL = "https://x" },
			"--client-id is required when --client-secret is set"},
		{"cloud URL needs --cloud", func(o *LoginOptions) { o.URL = cloud.DefaultGatewayURL + cloud.APIPath },
			"log in with --cloud instead"},
		{"--url conflicts with --cloud", func(o *LoginOptions) { o.Cloud = true; o.URL = "https://x" },
			"--url cannot be used with --cloud"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, out := jsonHarness(t, nil, "")
			tc.mutate(h.opts)
			err := runLogin(context.Background(), h.opts)
			var fe *cmdutil.FlagError
			if !errors.As(err, &fe) {
				t.Fatalf("err = %v, want a FlagError (exit code 2)", err)
			}
			if got := errorCode(t, out.String()); got != clierr.InvalidFlag {
				t.Errorf("code = %s", got)
			}
			if !strings.Contains(out.String(), tc.wantMsg) {
				t.Errorf("output %q missing %q", out.String(), tc.wantMsg)
			}
			if h.got != nil {
				t.Error("must not start authenticating on invalid flags")
			}
			if len(h.cfg.Instances) != 0 {
				t.Error("config must be untouched")
			}
		})
	}
}

func TestLogin_CloudUsesGatewayAndResourceIndicator(t *testing.T) {
	h, _ := jsonHarness(t, nil, "")
	h.opts.Cloud = true
	h.opts.Getenv = func(k string) string {
		if k == cloud.GatewayURLEnv {
			return "https://dev-gateway.example/"
		}
		return ""
	}
	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatal(err)
	}
	if h.got.URL != "https://dev-gateway.example"+cloud.APIPath || h.got.Resource != cloud.Resource {
		t.Errorf("authenticate got URL=%q resource=%q", h.got.URL, h.got.Resource)
	}
}

func TestLogin_StoredResourceAndAuthServerSurviveRelogin(t *testing.T) {
	h, _ := jsonHarness(t, map[string]config.Instance{
		"cloud": {URL: "https://gw/api", AuthServer: "https://idp", Auth: config.AuthConfig{Resource: cloud.Resource}},
	}, "cloud")
	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatal(err)
	}
	if h.got.Resource != cloud.Resource || h.got.AuthServer != "https://idp" {
		t.Errorf("got %+v", h.got)
	}
}

func TestLogin_AuthenticationFailureLeavesConfigAlone(t *testing.T) {
	h, out := jsonHarness(t, map[string]config.Instance{"work": {URL: "https://amp"}}, "work")
	h.opts.Authenticate = func(context.Context, auth.LoginOptions) (*config.Instance, error) {
		return nil, errors.New("authorization error: access_denied")
	}
	err := runLogin(context.Background(), h.opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := errorCode(t, out.String()); got != clierr.Transport || !strings.Contains(out.String(), "access_denied") {
		t.Errorf("out = %s", out.String())
	}
	if _, statErr := os.Stat(h.cfg.Path); !os.IsNotExist(statErr) {
		t.Error("a failed login must not write the config")
	}
}

func TestLogin_ConfigErrors(t *testing.T) {
	h, out := jsonHarness(t, nil, "")
	h.opts.Config = func() (*config.Config, error) { return nil, errors.New("corrupt yaml") }
	if err := runLogin(context.Background(), h.opts); err == nil || errorCode(t, out.String()) != clierr.ConfigNotLoaded {
		t.Fatalf("out = %s", out.String())
	}

	h2, out2 := jsonHarness(t, nil, "")
	h2.opts.URL = "https://amp"
	h2.cfg.Path = t.TempDir() // a directory: cannot be written
	if err := runLogin(context.Background(), h2.opts); err == nil || errorCode(t, out2.String()) != clierr.ConfigSaveFailed {
		t.Fatalf("out = %s", out2.String())
	}
}

func TestLogin_SingleOrgIsSelectedAndPersisted(t *testing.T) {
	h, out := jsonHarness(t, nil, "")
	h.opts.URL = "https://amp"
	h.opts.AgentManager = orgsClient(t, 200, "acme")
	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Instance string `json:"instance"`
		Org      string `json:"org"`
		Data     struct {
			URL           string           `json:"url"`
			OrgsAvailable []map[string]any `json:"orgs_available"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Instance != "default" || env.Org != "acme" || env.Data.URL != "https://amp" || len(env.Data.OrgsAvailable) != 1 {
		t.Errorf("envelope = %+v", env)
	}
	saved, err := config.Load(h.cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Instances["default"].CurrentOrg != "acme" || saved.CurrentInstance != "default" {
		t.Errorf("persisted = %+v", saved.Instances["default"])
	}
}

func TestLogin_OrgWarnings(t *testing.T) {
	cases := []struct {
		name   string
		client func(*testing.T) func(context.Context) (*amsvc.ClientWithResponses, error)
		want   string
		wantOr string
	}{
		{"several orgs", func(t *testing.T) func(context.Context) (*amsvc.ClientWithResponses, error) {
			return orgsClient(t, 200, "a", "b")
		}, "2 organizations available; pass --org", ""},
		{"no orgs", func(t *testing.T) func(context.Context) (*amsvc.ClientWithResponses, error) {
			return orgsClient(t, 200)
		}, "no organizations available", ""},
		{"org lookup rejected", func(t *testing.T) func(context.Context) (*amsvc.ClientWithResponses, error) {
			return orgsClient(t, 500)
		}, "failed to fetch organizations: BOOM: orgs unavailable", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := jsonHarness(t, nil, "")
			h.opts.URL = "https://amp"
			h.opts.AgentManager = tc.client(t)
			if err := runLogin(context.Background(), h.opts); err != nil {
				t.Fatalf("login must still succeed: %v", err)
			}
			if !strings.Contains(h.stderr(), tc.want) {
				t.Errorf("stderr = %q, want %q", h.stderr(), tc.want)
			}
			if got := h.cfg.Instances["default"].CurrentOrg; got != "" {
				t.Errorf("current org = %q, want none", got)
			}
		})
	}
}

func TestLogin_TextOutput(t *testing.T) {
	h := newLoginHarness(t, nil, "")
	h.opts.URL = "https://amp"
	h.opts.Name = "work"
	h.opts.AgentManager = orgsClient(t, 200, "acme")
	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Logged in to https://amp as work", "Organization set to acme"} {
		if !strings.Contains(h.stderr(), want) {
			t.Errorf("stderr missing %q: %q", want, h.stderr())
		}
	}
}

func TestLogin_SwitchingInstancesClearsLinks(t *testing.T) {
	linked := map[string]config.LinkedProject{"/work/repo": {Org: "o", Project: "p"}}

	t.Run("different instance name", func(t *testing.T) {
		h := newLoginHarness(t, map[string]config.Instance{"a": {URL: "https://a"}}, "a")
		h.cfg.LinkedProjects = map[string]config.LinkedProject{"/work/repo": linked["/work/repo"]}
		h.opts.URL, h.opts.Name = "https://b", "b"
		if err := runLogin(context.Background(), h.opts); err != nil {
			t.Fatal(err)
		}
		if len(h.cfg.LinkedProjects) != 0 {
			t.Errorf("links to the old instance survived: %v", h.cfg.LinkedProjects)
		}
		if !strings.Contains(h.stderr(), "Cleared 1 linked project(s)") {
			t.Errorf("stderr = %q", h.stderr())
		}
	})
	t.Run("same name, new URL", func(t *testing.T) {
		h := newLoginHarness(t, map[string]config.Instance{"a": {URL: "https://a"}}, "a")
		h.cfg.LinkedProjects = map[string]config.LinkedProject{"/work/repo": linked["/work/repo"]}
		h.opts.URL, h.opts.Name = "https://a-moved", "a"
		if err := runLogin(context.Background(), h.opts); err != nil {
			t.Fatal(err)
		}
		if len(h.cfg.LinkedProjects) != 0 {
			t.Errorf("links survived a URL change: %v", h.cfg.LinkedProjects)
		}
	})
	t.Run("re-login to the same instance keeps links", func(t *testing.T) {
		h := newLoginHarness(t, map[string]config.Instance{"a": {URL: "https://a"}}, "a")
		h.cfg.LinkedProjects = map[string]config.LinkedProject{"/work/repo": linked["/work/repo"]}
		if err := runLogin(context.Background(), h.opts); err != nil {
			t.Fatal(err)
		}
		if len(h.cfg.LinkedProjects) != 1 {
			t.Errorf("links lost on a plain re-login: %v", h.cfg.LinkedProjects)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func releasesClient(body string, status int) func() *http.Client {
	return func() *http.Client {
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})}
	}
}

func TestStartUpdateCheck(t *testing.T) {
	releases := `[{"tag_name":"amctl/v9.9.9","html_url":"https://example.com/rel","assets":[{"name":"amctl_v9.9.9_plan9_mips.zip","browser_download_url":"x"}]}]`
	cloudURL := cloud.DefaultGatewayURL + cloud.APIPath
	orig := version.Version
	t.Cleanup(func() { version.Version = orig })

	wait := func(ch <-chan string) string {
		select {
		case m := <-ch:
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("update check never reported")
			return ""
		}
	}

	t.Run("newer release is announced", func(t *testing.T) {
		version.Version = "1.0.0"
		ch := startUpdateCheck(context.Background(), &LoginOptions{URL: cloudURL, HTTPClient: releasesClient(releases, 200)})
		msg := wait(ch)
		if !strings.Contains(msg, "amctl 9.9.9 is available (you have 1.0.0)") || !strings.Contains(msg, "https://example.com/rel") {
			t.Errorf("msg = %q", msg)
		}
	})
	t.Run("already current", func(t *testing.T) {
		version.Version = "9.9.9"
		if msg := wait(startUpdateCheck(context.Background(), &LoginOptions{URL: cloudURL, HTTPClient: releasesClient(releases, 200)})); msg != "" {
			t.Errorf("msg = %q", msg)
		}
	})
	t.Run("lookup failure is silent", func(t *testing.T) {
		version.Version = "1.0.0"
		if msg := wait(startUpdateCheck(context.Background(), &LoginOptions{URL: cloudURL, HTTPClient: releasesClient("nope", 500)})); msg != "" {
			t.Errorf("msg = %q", msg)
		}
	})
	t.Run("never off cloud, for dev builds or without a client", func(t *testing.T) {
		version.Version = "1.0.0"
		if startUpdateCheck(context.Background(), &LoginOptions{URL: "https://self-hosted", HTTPClient: releasesClient(releases, 200)}) != nil {
			t.Error("self-hosted login must not phone home")
		}
		if startUpdateCheck(context.Background(), &LoginOptions{URL: cloudURL}) != nil {
			t.Error("no http client")
		}
		version.Version = "dev"
		if startUpdateCheck(context.Background(), &LoginOptions{URL: cloudURL, HTTPClient: releasesClient(releases, 200)}) != nil {
			t.Error("dev build must not check")
		}
	})
}

func TestLogin_CloudLoginPrintsUpdateNotice(t *testing.T) {
	orig := version.Version
	version.Version = "1.0.0"
	t.Cleanup(func() { version.Version = orig })

	h := newLoginHarness(t, nil, "")
	h.opts.Cloud = true
	h.opts.Getenv = func(string) string { return "" }
	h.opts.HTTPClient = releasesClient(`[{"tag_name":"amctl/v2.0.0","html_url":"https://example.com/rel","assets":[{"name":"amctl_v2.0.0_plan9_mips.zip","browser_download_url":"x"}]}]`, 200)
	h.opts.Authenticate = func(_ context.Context, o auth.LoginOptions) (*config.Instance, error) {
		time.Sleep(200 * time.Millisecond) // the check runs while the browser flow is open
		return &config.Instance{URL: o.URL}, nil
	}
	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stderr(), "amctl 2.0.0 is available") {
		t.Errorf("stderr = %q", h.stderr())
	}
}

func TestNewLoginCmd_FlagsAndWiring(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "c.yaml")
	cfg := &config.Config{Path: cfgPath}
	f := cmdutil.NewFactory(cfg, newLoginHarness(t, nil, "").opts.IO)
	c := NewLoginCmd(f)
	for _, name := range []string{"url", "name", "client-id", "client-secret", "auth-server", "cloud"} {
		if c.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s", name)
		}
	}
	// With no --url and nothing stored, RunE must fail with the first-login hint.
	c.SetContext(context.Background())
	if err := c.RunE(c, nil); err == nil {
		t.Fatal("expected first-login error")
	}
}
