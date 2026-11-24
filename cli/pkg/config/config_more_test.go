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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrent(t *testing.T) {
	c := &Config{Instances: map[string]Instance{"a": {URL: "https://a"}}}
	if _, err := c.Current(); err == nil || err.Error() != "no instance selected" {
		t.Errorf("err = %v", err)
	}
	c.CurrentInstance = "gone"
	if _, err := c.Current(); err == nil || err.Error() != `current instance "gone" not found in config` {
		t.Errorf("err = %v", err)
	}
	c.CurrentInstance = "a"
	inst, err := c.Current()
	if err != nil || inst.URL != "https://a" {
		t.Fatalf("inst=%v err=%v", inst, err)
	}
	// Current hands out a copy: edits must go back through the map.
	inst.URL = "mutated"
	if c.Instances["a"].URL != "https://a" {
		t.Error("Current exposes the stored instance")
	}
}

func TestAddInstanceSelectsIt(t *testing.T) {
	c := &Config{}
	c.AddInstance("a", Instance{URL: "https://a"})
	c.AddInstance("b", Instance{URL: "https://b"})
	if c.CurrentInstance != "b" || len(c.Instances) != 2 || c.Instances["a"].URL != "https://a" {
		t.Errorf("config = %+v", c)
	}
}

func TestSaveMethodWritesToConfigPathAndRoundTripsLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config")
	c := &Config{Path: path}
	c.AddInstance("a", Instance{URL: "https://a", CurrentOrg: "acme"})
	c.LinkProject("/repo", LinkedProject{Org: "acme", Project: "p", Agent: "bot"})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != path || got.CurrentInstance != "a" || got.Instances["a"].CurrentOrg != "acme" {
		t.Errorf("loaded = %+v", got)
	}
	if lp := got.LinkedProjects["/repo"]; lp.Project != "p" || lp.Agent != "bot" {
		t.Errorf("link = %+v", lp)
	}
}

func TestSaveFailsWhenPathIsADirectory(t *testing.T) {
	if err := Save(t.TempDir(), Config{}); err == nil {
		t.Error("saving onto a directory must fail")
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()

	missing, err := Load(filepath.Join(dir, "nope"))
	if err != nil || missing.Path == "" || len(missing.Instances) != 0 {
		t.Errorf("a missing file is an empty config, got %+v err %v", missing, err)
	}

	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(bad, []byte("instances: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Error("malformed YAML must fail")
	}

	// Typos in a hand-edited file are reported rather than silently dropped.
	typo := filepath.Join(dir, "typo")
	if err := os.WriteFile(typo, []byte("current_instanse: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(typo); err == nil {
		t.Error("unknown field must fail")
	}

	if _, err := Load(dir); err == nil {
		t.Error("a directory is not a config")
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	t.Setenv("USERPROFILE", "/home/someone")
	got, err := DefaultPath()
	if err != nil || filepath.ToSlash(got) != "/home/someone/.amctl/config" {
		t.Errorf("got %q err %v", got, err)
	}
}

func TestClearLinksIfSwitching_CountsAndKeeps(t *testing.T) {
	c := &Config{CurrentInstance: "a"}
	c.LinkProject("/x", LinkedProject{Org: "o"})
	c.LinkProject("/y", LinkedProject{Org: "o"})
	if n := c.ClearLinksIfSwitching("a"); n != 0 || len(c.LinkedProjects) != 2 {
		t.Errorf("same instance cleared %d links", n)
	}
	if n := c.ClearLinksIfSwitching("b"); n != 2 || len(c.LinkedProjects) != 0 {
		t.Errorf("switch cleared %d, left %v", n, c.LinkedProjects)
	}
}

func TestGetLinkedProject_ClosestAncestorWins(t *testing.T) {
	c := &Config{}
	c.LinkProject("/repo", LinkedProject{Project: "outer"})
	c.LinkProject("/repo/svc", LinkedProject{Project: "inner"})

	if dir, lp := c.GetLinkedProject("/repo/svc/pkg"); dir != "/repo/svc" || lp.Project != "inner" {
		t.Errorf("got %q %+v", dir, lp)
	}
	if dir, lp := c.GetLinkedProject("/repo/docs"); dir != "/repo" || lp.Project != "outer" {
		t.Errorf("got %q %+v", dir, lp)
	}
	if dir, lp := c.GetLinkedProject("/elsewhere"); dir != "" || lp != nil {
		t.Errorf("unlinked dir resolved to %q %+v", dir, lp)
	}
}
