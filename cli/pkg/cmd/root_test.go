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
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/version"
)

func newRoot(t *testing.T) (*cobra.Command, *iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	io, _, out, errOut := iostreams.Test()
	cfg := &config.Config{Path: t.TempDir() + "/config.yaml", Instances: map[string]config.Instance{}}
	root, err := NewRootCmd(cmdutil.NewFactory(cfg, io))
	if err != nil {
		t.Fatal(err)
	}
	root.SetOut(out)
	root.SetErr(errOut)
	return root, io, out, errOut
}

func TestNewRootCmd_CommandTree(t *testing.T) {
	root, _, _, _ := newRoot(t)

	visible := map[string]bool{}
	hidden := map[string]bool{}
	for _, c := range root.Commands() {
		if c.Hidden {
			hidden[c.Name()] = true
		} else {
			visible[c.Name()] = true
		}
	}
	for _, want := range []string{"login", "agent", "api", "context", "gateway", "llm-provider", "project", "version", "skills"} {
		if !visible[want] {
			t.Errorf("missing top-level command %q (have %v)", want, visible)
		}
	}
	// `amctl link` / `unlink` stay as hidden shortcuts for `amctl context link`.
	for _, want := range []string{"link", "unlink"} {
		if !hidden[want] {
			t.Errorf("%q must exist as a hidden alias", want)
		}
	}
	if !root.SilenceErrors || !root.SilenceUsage {
		t.Error("root must leave error rendering to amcmd.Main")
	}
	if root.PersistentFlags().Lookup("org") == nil || root.PersistentFlags().Lookup("json") == nil {
		t.Error("--org and --json must be persistent flags")
	}
	if !strings.HasPrefix(root.Version, version.Version) {
		t.Errorf("Version = %q", root.Version)
	}
}

func TestNewRootCmd_JSONFlagTogglesIOStreams(t *testing.T) {
	root, io, out, _ := newRoot(t)
	root.SetArgs([]string{"version", "--json"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if !io.JSON {
		t.Error("--json did not reach the IOStreams the commands render with")
	}
	if !strings.Contains(out.String(), version.String()) {
		t.Errorf("version output = %q", out.String())
	}
}

func TestNewRootCmd_BadFlagsAreFlagErrors(t *testing.T) {
	root, _, _, _ := newRoot(t)
	root.SetArgs([]string{"version", "--nope"})
	_, err := root.ExecuteC()
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FlagError so the process exits 2", err)
	}
	if !strings.Contains(err.Error(), "unknown flag: --nope") {
		t.Errorf("message = %q", err)
	}
}

func TestNewRootCmd_UnknownCommandFails(t *testing.T) {
	root, _, _, _ := newRoot(t)
	root.SetArgs([]string{"frobnicate"})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("unknown command must fail")
	}
}

func TestVersionCmd(t *testing.T) {
	orig := version.Version
	version.Version = "1.2.3"
	t.Cleanup(func() { version.Version = orig })

	c := NewVersionCmd()
	out := &bytes.Buffer{}
	c.SetOut(out)
	c.SetArgs(nil)
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "amctl version 1.2.3 (commit none, built unknown)\n" {
		t.Errorf("output = %q", got)
	}

	c2 := NewVersionCmd()
	c2.SetArgs([]string{"extra"})
	c2.SetOut(&bytes.Buffer{})
	c2.SetErr(&bytes.Buffer{})
	if err := c2.Execute(); err == nil {
		t.Error("version takes no arguments")
	}
}

func TestDisableFileCompletion_CoversWholeTree(t *testing.T) {
	root, _, _, _ := newRoot(t)

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.ValidArgsFunction == nil {
			t.Errorf("%s: file completion still enabled for args", c.CommandPath())
		}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if _, ok := c.GetFlagCompletionFunc(f.Name); !ok {
				t.Errorf("%s --%s: file completion still enabled", c.CommandPath(), f.Name)
			}
		})
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)

	// An explicit ValidArgsFunction is kept, not overwritten.
	custom := &cobra.Command{Use: "x", ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"mine"}, cobra.ShellCompDirectiveNoFileComp
	}}
	disableFileCompletion(custom)
	if got, _ := custom.ValidArgsFunction(custom, nil, ""); len(got) != 1 || got[0] != "mine" {
		t.Errorf("custom completion replaced: %v", got)
	}
}
