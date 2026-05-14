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
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgskills "github.com/wso2/agent-manager/cli/pkg/skills"
)

func TestRemoveCmd_TextOutput(t *testing.T) {
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

	io, _, errOut := newTextIO()
	err = runRemove(context.Background(), &RemoveOptions{
		IO:       io,
		DestDir:  dest,
		ToolDirs: []string{toolDir},
	})
	if err != nil {
		t.Fatalf("runRemove failed: %v", err)
	}

	output := errOut.String()
	if !strings.Contains(output, "use-amctl") {
		t.Errorf("expected output to mention use-amctl, got:\n%s", output)
	}

	if _, err := os.Stat(filepath.Join(dest, "use-amctl")); !os.IsNotExist(err) {
		t.Error("skill dir should be removed")
	}
}

func TestRemoveCmd_NothingInstalled(t *testing.T) {
	dest := t.TempDir()

	io, _, errOut := newTextIO()
	err := runRemove(context.Background(), &RemoveOptions{
		IO:      io,
		DestDir: dest,
	})
	if err != nil {
		t.Fatalf("runRemove failed: %v", err)
	}

	output := errOut.String()
	if !strings.Contains(output, "No skills installed") {
		t.Errorf("expected 'No skills installed' message, got:\n%s", output)
	}
}

func TestRemoveCmd_JSONNoTextOutput(t *testing.T) {
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

	io, _, errOut := newTestIO(true)
	err = runRemove(context.Background(), &RemoveOptions{
		IO:       io,
		DestDir:  dest,
		ToolDirs: []string{toolDir},
	})
	if err != nil {
		t.Fatalf("runRemove failed: %v", err)
	}

	if errOut.Len() > 0 {
		t.Errorf("expected no stderr output in JSON mode, got:\n%s", errOut.String())
	}
}
