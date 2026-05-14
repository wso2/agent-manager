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
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
)

func TestResolveAgent_FromArgs(t *testing.T) {
	f := &Factory{
		Config: func() (*config.Config, error) {
			return &config.Config{
				LinkedProjects: map[string]config.LinkedProject{
					"/some/dir": {Org: "o", Project: "p", Agent: "linked-agent"},
				},
			}, nil
		},
	}
	agent, remaining, err := f.ResolveAgent([]string{"explicit-agent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent != "explicit-agent" {
		t.Errorf("agent = %q, want %q", agent, "explicit-agent")
	}
	if len(remaining) != 0 {
		t.Errorf("remaining = %v, want empty", remaining)
	}
}

func TestResolveAgent_FromLinkedContext(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	f := &Factory{
		Config: func() (*config.Config, error) {
			return &config.Config{
				LinkedProjects: map[string]config.LinkedProject{
					wd: {Org: "o", Project: "p", Agent: "linked-agent"},
				},
			}, nil
		},
	}
	agent, remaining, err := f.ResolveAgent(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent != "linked-agent" {
		t.Errorf("agent = %q, want %q", agent, "linked-agent")
	}
	if remaining != nil {
		t.Errorf("remaining = %v, want nil", remaining)
	}
}

func TestResolveAgent_NoAgentAvailable(t *testing.T) {
	f := &Factory{
		Config: func() (*config.Config, error) {
			return &config.Config{}, nil
		},
	}
	_, _, err := f.ResolveAgent(nil)
	if err == nil {
		t.Fatal("expected error when no agent available")
	}
	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error type = %T, want clierr.CLIError", err)
	}
	if cliErr.Code != clierr.NoAgent {
		t.Errorf("code = %q, want %q", cliErr.Code, clierr.NoAgent)
	}
}

func TestResolveAgent_RemainingArgs(t *testing.T) {
	f := &Factory{
		Config: func() (*config.Config, error) {
			return &config.Config{}, nil
		},
	}
	agent, remaining, err := f.ResolveAgent([]string{"my-agent", "build-001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent != "my-agent" {
		t.Errorf("agent = %q, want %q", agent, "my-agent")
	}
	if len(remaining) != 1 || remaining[0] != "build-001" {
		t.Errorf("remaining = %v, want [\"build-001\"]", remaining)
	}
}

func TestResolveAgent_EmptyStringArgFallsThrough(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	f := &Factory{
		Config: func() (*config.Config, error) {
			return &config.Config{
				LinkedProjects: map[string]config.LinkedProject{
					wd: {Org: "o", Project: "p", Agent: "linked-agent"},
				},
			}, nil
		},
	}
	agent, remaining, err := f.ResolveAgent([]string{""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent != "linked-agent" {
		t.Errorf("agent = %q, want %q", agent, "linked-agent")
	}
	if len(remaining) != 1 || remaining[0] != "" {
		t.Errorf("remaining = %v, want [\"\"]", remaining)
	}
}

func TestResolveEnvironment_FromFlag(t *testing.T) {
	cfg := &config.Config{}
	f := &Factory{Config: func() (*config.Config, error) { return cfg, nil }}

	cmd := &cobra.Command{}
	cmd.Flags().String("env", "", "")
	if err := cmd.Flags().Set("env", "production"); err != nil {
		t.Fatalf("set --env: %v", err)
	}

	env, err := f.ResolveEnvironment(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env != "production" {
		t.Errorf("env = %q, want %q", env, "production")
	}
}

func TestResolveEnvironment_FromLinkedContext(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cfg := &config.Config{
		LinkedProjects: map[string]config.LinkedProject{
			wd: {Org: "acme", Project: "p", Environment: "staging"},
		},
	}
	f := &Factory{Config: func() (*config.Config, error) { return cfg, nil }}

	cmd := &cobra.Command{}
	cmd.Flags().String("env", "", "")

	env, err := f.ResolveEnvironment(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env != "staging" {
		t.Errorf("env = %q, want %q", env, "staging")
	}
}

func TestResolveEnvironment_MissingReturnsError(t *testing.T) {
	cfg := &config.Config{}
	f := &Factory{Config: func() (*config.Config, error) { return cfg, nil }}

	cmd := &cobra.Command{}
	cmd.Flags().String("env", "", "")

	_, err := f.ResolveEnvironment(cmd)
	if err == nil {
		t.Fatal("expected error for missing environment")
	}
	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error type = %T, want clierr.CLIError", err)
	}
	if cliErr.Code != clierr.NoEnvironment {
		t.Errorf("code = %q, want %q", cliErr.Code, clierr.NoEnvironment)
	}
}
