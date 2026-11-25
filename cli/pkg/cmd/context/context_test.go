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

package context

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// workdir moves the process into a fresh directory (the commands link the
// current directory) and returns its resolved path.
func workdir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func newConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Path:            filepath.Join(t.TempDir(), "config.yaml"),
		CurrentInstance: "work",
		Instances:       map[string]config.Instance{"work": {URL: "https://amp.example.com", CurrentOrg: "acme"}},
	}
}

func jsonIO() (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.JSON = true
	return io, out, errOut
}

func textIO() (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	return io, out, errOut
}

func envelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("not JSON: %v\n%q", err, raw)
	}
	return m
}

func errCode(t *testing.T, raw string) string {
	t.Helper()
	e, ok := envelope(t, raw)["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in envelope: %s", raw)
	}
	return e["code"].(string)
}

func cfgFn(cfg *config.Config) func() (*config.Config, error) {
	return func() (*config.Config, error) { return cfg, nil }
}

func failingCfg() (*config.Config, error) { return nil, errors.New("unreadable") }

// platform answers project and agent lookups; names in notFound return 404.
func platform(t *testing.T, notFound ...string) func(context.Context) (*amsvc.ClientWithResponses, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for _, nf := range notFound {
			if strings.HasSuffix(r.URL.Path, "/"+nf) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"` + nf + ` not found"}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"name":"x","displayName":"X","deploymentPipeline":"default","createdAt":"2026-01-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)
	c, err := amsvc.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return c, nil }
}

func linkOpts(io *iostreams.IOStreams, cfg *config.Config, client func(context.Context) (*amsvc.ClientWithResponses, error), agent string) *LinkOptions {
	return &LinkOptions{IO: io, Config: cfgFn(cfg), Client: client, Agent: agent}
}

func TestLink_PersistsAndIsReadBackByShow(t *testing.T) {
	dir := workdir(t)
	cfg := newConfig(t)
	io, out, _ := jsonIO()

	err := runLink(context.Background(), linkOpts(io, cfg, platform(t), "bot"), "acme", "proj", render.Scope{Instance: "work", Org: "acme", Project: "proj"})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	data := envelope(t, out.String())["data"].(map[string]any)
	if data["dir"] != dir || data["org"] != "acme" || data["project"] != "proj" || data["agent"] != "bot" {
		t.Errorf("link result = %v", data)
	}

	// The link is on disk, not just in memory.
	saved, err := config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	if lp := saved.LinkedProjects[dir]; lp.Org != "acme" || lp.Project != "proj" || lp.Agent != "bot" {
		t.Errorf("persisted link = %+v", lp)
	}

	// A subdirectory resolves to the same link through `show`.
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	sio, sout, _ := jsonIO()
	if err := runShow(&ShowOptions{IO: sio, Config: cfgFn(saved)}); err != nil {
		t.Fatal(err)
	}
	sd := envelope(t, sout.String())["data"].(map[string]any)
	linked := sd["linked"].(map[string]any)
	if sd["url"] != "https://amp.example.com" || sd["org"] != "acme" || linked["project"] != "proj" || linked["agent"] != "bot" {
		t.Errorf("show = %v", sd)
	}
}

func TestLink_TextOutput(t *testing.T) {
	dir := workdir(t)
	io, _, errOut := textIO()
	if err := runLink(context.Background(), linkOpts(io, newConfig(t), platform(t), ""), "acme", "proj", render.Scope{}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Linked", dir, "acme", "proj"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr missing %q: %q", want, errOut.String())
		}
	}
	if strings.Contains(errOut.String(), "agent:") {
		t.Error("no agent was given")
	}

	io2, _, errOut2 := textIO()
	if err := runLink(context.Background(), linkOpts(io2, newConfig(t), platform(t), "bot"), "acme", "proj", render.Scope{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut2.String(), "(agent: bot)") {
		t.Errorf("stderr = %q", errOut2.String())
	}
}

func TestLink_FailuresLeaveConfigUntouched(t *testing.T) {
	workdir(t)
	cases := []struct {
		name     string
		opts     func(io *iostreams.IOStreams, cfg *config.Config) *LinkOptions
		project  string
		wantCode string
	}{
		{"unknown project", func(io *iostreams.IOStreams, cfg *config.Config) *LinkOptions {
			return linkOpts(io, cfg, platform(t, "ghost"), "")
		}, "ghost", "NOT_FOUND"},
		{"unknown agent", func(io *iostreams.IOStreams, cfg *config.Config) *LinkOptions {
			return linkOpts(io, cfg, platform(t, "ghost-agent"), "ghost-agent")
		}, "proj", "NOT_FOUND"},
		{"invalid project name", func(io *iostreams.IOStreams, cfg *config.Config) *LinkOptions {
			return linkOpts(io, cfg, platform(t), "")
		}, "a/b", clierr.InvalidFlag},
		{"invalid agent name", func(io *iostreams.IOStreams, cfg *config.Config) *LinkOptions {
			return linkOpts(io, cfg, platform(t), "../x")
		}, "proj", clierr.InvalidFlag},
		{"client unavailable", func(io *iostreams.IOStreams, cfg *config.Config) *LinkOptions {
			return linkOpts(io, cfg, func(context.Context) (*amsvc.ClientWithResponses, error) {
				return nil, clierr.New(clierr.NoInstance, "no instance")
			}, "")
		}, "proj", clierr.NoInstance},
		{"config unreadable", func(io *iostreams.IOStreams, cfg *config.Config) *LinkOptions {
			o := linkOpts(io, cfg, platform(t), "")
			o.Config = failingCfg
			return o
		}, "proj", clierr.ConfigNotLoaded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newConfig(t)
			io, out, _ := jsonIO()
			err := runLink(context.Background(), tc.opts(io, cfg), "acme", tc.project, render.Scope{})
			if err == nil {
				t.Fatal("expected error")
			}
			if got := errCode(t, out.String()); got != tc.wantCode {
				t.Errorf("code = %s, want %s", got, tc.wantCode)
			}
			if len(cfg.LinkedProjects) != 0 {
				t.Errorf("failed link must not be recorded: %v", cfg.LinkedProjects)
			}
		})
	}
}

func TestLink_TransportFailure(t *testing.T) {
	workdir(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c, _ := amsvc.NewClientWithResponses(srv.URL)
	io, out, _ := jsonIO()
	err := runLink(context.Background(), linkOpts(io, newConfig(t), func(context.Context) (*amsvc.ClientWithResponses, error) { return c, nil }, ""),
		"acme", "proj", render.Scope{})
	if err == nil || errCode(t, out.String()) != clierr.Transport {
		t.Fatalf("err=%v out=%s", err, out.String())
	}
}

func TestLink_ConfigSaveFailure(t *testing.T) {
	workdir(t)
	cfg := newConfig(t)
	cfg.Path = t.TempDir() // a directory: the file cannot be written
	io, out, _ := jsonIO()
	err := runLink(context.Background(), linkOpts(io, cfg, platform(t), ""), "acme", "proj", render.Scope{})
	if err == nil || errCode(t, out.String()) != clierr.ConfigSaveFailed {
		t.Fatalf("err=%v out=%s", err, out.String())
	}
}

func TestUnlink(t *testing.T) {
	dir := workdir(t)
	cfg := newConfig(t)
	cfg.LinkProject(dir, config.LinkedProject{Org: "acme", Project: "proj"})
	sub := filepath.Join(dir, "deep")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	io, out, _ := jsonIO()
	if err := runUnlink(&UnlinkOptions{IO: io, Config: cfgFn(cfg)}); err != nil {
		t.Fatal(err)
	}
	if got := envelope(t, out.String())["data"].(map[string]any)["dir"]; got != dir {
		t.Errorf("unlinked dir = %v, want the ancestor %s that held the link", got, dir)
	}
	saved, _ := config.Load(cfg.Path)
	if len(saved.LinkedProjects) != 0 {
		t.Errorf("link still on disk: %v", saved.LinkedProjects)
	}

	// Second time: nothing is linked any more.
	io2, out2, _ := jsonIO()
	if err := runUnlink(&UnlinkOptions{IO: io2, Config: cfgFn(cfg)}); err == nil || errCode(t, out2.String()) != clierr.NotLinked {
		t.Fatalf("err=%v out=%s", err, out2.String())
	}
}

func TestUnlink_TextAndFailures(t *testing.T) {
	dir := workdir(t)
	cfg := newConfig(t)
	cfg.LinkProject(dir, config.LinkedProject{Org: "acme", Project: "proj"})
	io, _, errOut := textIO()
	if err := runUnlink(&UnlinkOptions{IO: io, Config: cfgFn(cfg)}); err != nil || !strings.Contains(errOut.String(), "Unlinked") {
		t.Fatalf("err=%v stderr=%q", err, errOut.String())
	}

	jio, out, _ := jsonIO()
	if err := runUnlink(&UnlinkOptions{IO: jio, Config: failingCfg}); err == nil || errCode(t, out.String()) != clierr.ConfigNotLoaded {
		t.Fatalf("out=%s", out.String())
	}

	bad := newConfig(t)
	bad.LinkProject(dir, config.LinkedProject{Org: "a", Project: "p"})
	bad.Path = t.TempDir()
	jio2, out2, _ := jsonIO()
	if err := runUnlink(&UnlinkOptions{IO: jio2, Config: cfgFn(bad)}); err == nil || errCode(t, out2.String()) != clierr.ConfigSaveFailed {
		t.Fatalf("out=%s", out2.String())
	}
}

func TestShow(t *testing.T) {
	dir := workdir(t)

	t.Run("text with link", func(t *testing.T) {
		cfg := newConfig(t)
		cfg.LinkProject(dir, config.LinkedProject{Org: "acme", Project: "proj", Agent: "bot"})
		io, out, _ := textIO()
		if err := runShow(&ShowOptions{IO: io, Config: cfgFn(cfg)}); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"work", "https://amp.example.com", "acme", "linked project:", "proj", "bot"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("output missing %q:\n%s", want, out.String())
			}
		}
	})
	t.Run("text without link or org", func(t *testing.T) {
		cfg := newConfig(t)
		cfg.Instances["work"] = config.Instance{URL: "https://amp.example.com"}
		io, out, _ := textIO()
		if err := runShow(&ShowOptions{IO: io, Config: cfgFn(cfg)}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "linked project") || strings.Contains(out.String(), "org:") {
			t.Errorf("unexpected sections:\n%s", out.String())
		}
	})
	t.Run("json omits an absent link", func(t *testing.T) {
		io, out, _ := jsonIO()
		if err := runShow(&ShowOptions{IO: io, Config: cfgFn(newConfig(t))}); err != nil {
			t.Fatal(err)
		}
		env := envelope(t, out.String())
		if _, has := env["data"].(map[string]any)["linked"]; has {
			t.Errorf("linked present: %v", env["data"])
		}
		if env["instance"] != "work" {
			t.Errorf("instance = %v", env["instance"])
		}
	})
	t.Run("no instance", func(t *testing.T) {
		io, out, _ := jsonIO()
		if err := runShow(&ShowOptions{IO: io, Config: cfgFn(&config.Config{})}); err == nil || errCode(t, out.String()) != clierr.NoInstance {
			t.Fatalf("out=%s", out.String())
		}
	})
	t.Run("dangling current instance", func(t *testing.T) {
		io, out, _ := jsonIO()
		cfg := &config.Config{CurrentInstance: "gone", Instances: map[string]config.Instance{}}
		err := runShow(&ShowOptions{IO: io, Config: cfgFn(cfg)})
		if err == nil || errCode(t, out.String()) != clierr.NoInstance || !strings.Contains(out.String(), `\"gone\"`) {
			t.Fatalf("out=%s", out.String())
		}
	})
	t.Run("config unreadable", func(t *testing.T) {
		io, out, _ := jsonIO()
		if err := runShow(&ShowOptions{IO: io, Config: failingCfg}); err == nil || errCode(t, out.String()) != clierr.ConfigNotLoaded {
			t.Fatalf("out=%s", out.String())
		}
	})
}
