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

package skills

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkgskills "github.com/wso2/agent-manager/cli/pkg/skills"
)

func TestListCmd_WithInstalledSkills(t *testing.T) {
	dest := t.TempDir()
	toolDir := t.TempDir()

	fetch := fakeFetchFS()
	fsys, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkgskills.Install(context.Background(), fsys, dest, []string{toolDir}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	io, out, _ := newTextIO()
	err = runList(context.Background(), &ListOptions{
		IO:       io,
		DestDir:  dest,
		ToolDirs: []string{toolDir},
		FetchFS:  fetch,
	})
	if err != nil {
		t.Fatalf("runList failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "use-amctl") {
		t.Errorf("expected output to mention use-amctl, got:\n%s", output)
	}
}

func TestListCmd_NothingInstalled(t *testing.T) {
	dest := t.TempDir()

	io, out, _ := newTextIO()
	err := runList(context.Background(), &ListOptions{
		IO:      io,
		DestDir: dest,
		FetchFS: fakeFetchFS(),
	})
	if err != nil {
		t.Fatalf("runList failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "use-amctl") {
		t.Errorf("expected output to mention use-amctl (from remote, not installed), got:\n%s", output)
	}
	if !strings.Contains(output, "not installed") {
		t.Errorf("expected '(not installed)' tag, got:\n%s", output)
	}
}

func TestListCmd_JSONOutput(t *testing.T) {
	dest := t.TempDir()

	fetch := fakeFetchFS()
	fsys, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkgskills.Install(context.Background(), fsys, dest, nil); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	io, out, _ := newTestIO(true)
	err = runList(context.Background(), &ListOptions{
		IO:      io,
		DestDir: dest,
		FetchFS: fetch,
	})
	if err != nil {
		t.Fatalf("runList failed: %v", err)
	}

	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\nbody=%s", err, out.String())
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'data' key, got %v", env)
	}
	skills, ok := data["skills"].([]any)
	if !ok || len(skills) == 0 {
		t.Errorf("expected skills array in data, got %v", data)
	}
}
