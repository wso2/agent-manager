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
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func newTestIO(canPrompt bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.SetTerminal(canPrompt, canPrompt, canPrompt)
	io.JSON = true
	return io, out, errOut
}

func newTextIO() (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	return io, out, errOut
}

func fakeFetchFS() func(ctx context.Context) (fs.FS, error) {
	return func(ctx context.Context) (fs.FS, error) {
		return fstest.MapFS{
			"skilldata/use-amctl/SKILL.md": &fstest.MapFile{
				Data: []byte("---\nname: use-amctl\ndescription: test description\n---\n\nbody"),
				Mode: 0o644,
			},
		}, nil
	}
}

func TestInstallCmd_TextOutput(t *testing.T) {
	dest := t.TempDir()
	toolDir := filepath.Join(t.TempDir(), ".claude", "skills")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Dir(filepath.Dir(toolDir))

	io, _, errOut := newTextIO()
	err := runInstall(context.Background(), &InstallOptions{
		IO:      io,
		HomeDir: home,
		DestDir: dest,
		FetchFS: fakeFetchFS(),
	})
	if err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	output := errOut.String()
	if !strings.Contains(output, "use-amctl") {
		t.Errorf("expected output to mention use-amctl, got:\n%s", output)
	}
	if !strings.Contains(output, "Extracted") {
		t.Errorf("expected output to contain 'Extracted', got:\n%s", output)
	}
}

func TestInstallCmd_JSONOutput(t *testing.T) {
	dest := t.TempDir()

	io, out, _ := newTestIO(true)
	err := runInstall(context.Background(), &InstallOptions{
		IO:      io,
		HomeDir: t.TempDir(),
		DestDir: dest,
		FetchFS: fakeFetchFS(),
	})
	if err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\nbody=%s", err, out.String())
	}
	if _, ok := env["instance"]; !ok {
		t.Error("expected 'instance' key in JSON envelope")
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

func TestInstallCmd_JSONNoTextOutput(t *testing.T) {
	dest := t.TempDir()

	io, _, errOut := newTestIO(true)
	err := runInstall(context.Background(), &InstallOptions{
		IO:      io,
		HomeDir: t.TempDir(),
		DestDir: dest,
		FetchFS: fakeFetchFS(),
	})
	if err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	if errOut.Len() > 0 {
		t.Errorf("expected no stderr output in JSON mode, got:\n%s", errOut.String())
	}
}
