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

package cmdutil

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
)

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 24 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"2h30m", 150 * time.Minute, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"0d", 0, true},
		{"-1d", 0, true},
		{"xd", 0, true},
		{"0s", 0, true},
		{"-5m", 0, true},
		{"banana", 0, true},
	}
	for _, c := range cases {
		got, err := ParseDuration(c.in)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v (err=%v)", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func TestResolveSinceWindow(t *testing.T) {
	start, end, err := ResolveSinceWindow("2h")
	if err != nil {
		t.Fatal(err)
	}
	s, err1 := time.Parse(time.RFC3339, start)
	e, err2 := time.Parse(time.RFC3339, end)
	if err1 != nil || err2 != nil {
		t.Fatalf("not RFC3339: %q %q", start, end)
	}
	if e.Sub(s) != 2*time.Hour {
		t.Errorf("window = %v, want 2h", e.Sub(s))
	}
	if time.Since(e) > time.Minute {
		t.Errorf("window end %v is not now", e)
	}
	if _, _, err := ResolveSinceWindow("nonsense"); err == nil {
		t.Error("invalid --since must fail")
	}
}

func TestFlagErrorWrap(t *testing.T) {
	plain := FlagErrorWrap(errors.New("unknown flag: --nope"))
	var fe *FlagError
	var ce clierr.CLIError
	if !errors.As(plain, &fe) || !errors.As(plain, &ce) || ce.Code != clierr.InvalidFlag || ce.Message != "unknown flag: --nope" {
		t.Fatalf("plain error: %v (%+v)", plain, ce)
	}

	// An existing CLIError keeps its code (wire contract) but gains exit-2 semantics.
	kept := FlagErrorWrap(clierr.New(clierr.Validation, "bad"))
	if !errors.As(kept, &fe) || !errors.As(kept, &ce) || ce.Code != clierr.Validation {
		t.Fatalf("clierr: %v", kept)
	}
	if FlagErrorf("x %d", 1).Error() != "INVALID_FLAG: x 1" {
		t.Errorf("FlagErrorf = %q", FlagErrorf("x %d", 1))
	}
}

func TestObserverErrorFromResponse(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
		wantStat int
	}{
		{"transport", errors.New("dial tcp: refused"), clierr.Transport, "dial tcp: refused", 0},
		{"401", &observersvc.HTTPError{StatusCode: 401}, clierr.Unauthorized, "observer returned 401", 401},
		{"403", &observersvc.HTTPError{StatusCode: 403}, clierr.Forbidden, "observer returned 403", 403},
		{"404 with body", &observersvc.HTTPError{StatusCode: 404, Body: &observersvc.ErrorResponse{Message: "trace not found"}}, clierr.NotFound, "trace not found", 404},
		{"400", &observersvc.HTTPError{StatusCode: 400}, clierr.Validation, "observer returned 400", 400},
		{"500", &observersvc.HTTPError{StatusCode: 500}, clierr.ServerError, "observer returned 500", 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ObserverErrorFromResponse(c.err)
			if got.Code != c.wantCode || got.Message != c.wantMsg || got.Status != c.wantStat {
				t.Errorf("got %+v", got)
			}
			if got.AdditionalData == nil {
				t.Error("AdditionalData must be non-nil so JSON emits {}")
			}
		})
	}
}

func TestScopeBuilders(t *testing.T) {
	f := &Factory{Config: func() (*config.Config, error) {
		return &config.Config{CurrentInstance: "work"}, nil
	}}
	s := f.Scope("acme", "proj")
	if s.Instance != "work" || s.Org != "acme" || s.Project != "proj" || s.Agent != "" {
		t.Errorf("Scope = %+v", s)
	}
	if a := f.AgentScope("acme", "proj", "bot"); a.Agent != "bot" || a.Instance != "work" {
		t.Errorf("AgentScope = %+v", a)
	}

	broken := &Factory{Config: func() (*config.Config, error) { return nil, errors.New("x") }}
	if s := broken.Scope("o", "p"); s.Instance != "" || s.Org != "o" {
		t.Errorf("Scope with broken config = %+v", s)
	}
}

func TestResolveOrgProject_FallsBackToCurrentOrg(t *testing.T) {
	cfg := &config.Config{CurrentInstance: "w", Instances: map[string]config.Instance{"w": {CurrentOrg: "home"}}}
	f := &Factory{Config: func() (*config.Config, error) { return cfg, nil }}
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("org", "", "")
	cmd.Flags().String("project", "", "")

	org, project, err := f.ResolveOrgProject(cmd, true, false)
	if err != nil || org != "home" || project != "" {
		t.Fatalf("org=%q project=%q err=%v", org, project, err)
	}
	_ = cmd.Flags().Set("org", "flagged")
	if org, _, _ := f.ResolveOrgProject(cmd, true, false); org != "flagged" {
		t.Errorf("flag must beat current_org, got %q", org)
	}

	var ce clierr.CLIError
	if _, _, err := f.ResolveOrgProject(cmd, true, true); !errors.As(err, &ce) || ce.Code != clierr.NoProject {
		t.Errorf("missing project err = %v", err)
	}
	empty := &Factory{Config: func() (*config.Config, error) { return &config.Config{}, nil }}
	cmd2 := &cobra.Command{Use: "y"}
	cmd2.Flags().String("org", "", "")
	if _, _, err := empty.ResolveOrgProject(cmd2, true, false); !errors.As(err, &ce) || ce.Code != clierr.NoOrg {
		t.Errorf("missing org err = %v", err)
	}
}

func TestResolveAgent_NothingToResolve(t *testing.T) {
	f := &Factory{Config: func() (*config.Config, error) { return &config.Config{}, nil }}
	_, _, err := f.ResolveAgent(nil)
	var ce clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != clierr.NoAgent {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := f.ResolveAgent([]string{""}); err == nil {
		t.Error("empty arg must not count as an agent name")
	}
}

func TestEnableOverrides_RegisterFlagsAndCompletion(t *testing.T) {
	p := newFakePlatform(t)
	f, _ := factoryFor(t, p, config.AuthConfig{AccessToken: "t"})
	root := &cobra.Command{Use: "amctl"}
	EnableOrgOverride(root, f)
	EnableProjectOverride(root, f)
	for _, name := range []string{"org", "project"} {
		if root.PersistentFlags().Lookup(name) == nil {
			t.Errorf("--%s not registered", name)
		}
	}

	// The registered completion hits the API through the factory.
	out, directive := completeFlag(t, root, "org")
	if !reflect.DeepEqual(out, []string{"acme"}) || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("--org completion = %v (%v)", out, directive)
	}
}

// completeFlag looks up and runs the completion func registered for --<flag>.
func completeFlag(t *testing.T, root *cobra.Command, flag string) ([]string, cobra.ShellCompDirective) {
	t.Helper()
	leaf := &cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(leaf)
	fn, ok := leaf.GetFlagCompletionFunc(flag)
	if !ok {
		t.Fatalf("no completion registered for --%s", flag)
	}
	leaf.SetContext(context.Background())
	return fn(leaf, nil, "")
}

func TestCompleteLLMProvidersTemplatesGateways(t *testing.T) {
	p := newFakePlatform(t)
	f, _ := factoryFor(t, p, config.AuthConfig{AccessToken: "t"})
	f.Config = func() (*config.Config, error) {
		return &config.Config{CurrentInstance: "work", Instances: map[string]config.Instance{
			"work": {URL: p.URL, CurrentOrg: "acme", Auth: config.AuthConfig{AccessToken: "t"}}}}, nil
	}
	cmd := newCmdWithCtx(context.Background())
	cmd.Flags().String("org", "", "")

	if got := CompleteLLMProviders(cmd, f); !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Errorf("CompleteLLMProviders = %v (want sorted)", got)
	}
	// The fake platform has no template or gateway routes: 404 => nil, not a panic.
	if got := CompleteLLMProviderTemplates(cmd, f); got != nil {
		t.Errorf("templates on 404 = %v", got)
	}
	if got := CompleteGateways(cmd, f); got != nil {
		t.Errorf("gateways on 404 = %v", got)
	}
}

func TestCompleteAIGatewayCompletions_NoOrgOrNoClient(t *testing.T) {
	cmd := newCmdWithCtx(context.Background())
	cmd.Flags().String("org", "", "")
	noOrg := &Factory{Config: func() (*config.Config, error) { return &config.Config{}, nil }}
	for name, fn := range map[string]func(*cobra.Command, *Factory) []string{
		"providers": CompleteLLMProviders, "templates": CompleteLLMProviderTemplates, "gateways": CompleteGateways,
	} {
		if got := fn(cmd, noOrg); got != nil {
			t.Errorf("%s without org = %v", name, got)
		}
	}
}
