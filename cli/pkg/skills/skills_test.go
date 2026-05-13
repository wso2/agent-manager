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
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSkills(t *testing.T) {
	names := EmbeddedSkills()
	if len(names) == 0 {
		t.Fatal("expected at least one embedded skill")
	}
	found := false
	for _, n := range names {
		if n == "use-amctl" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected use-amctl in embedded skills, got %v", names)
	}
}

func TestDetectToolDirs_FindsExisting(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dirs := DetectToolDirs(home)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != claudeDir {
		t.Errorf("dir = %q, want %q", dirs[0], claudeDir)
	}
}

func TestDetectToolDirs_SkipsMissing(t *testing.T) {
	home := t.TempDir()
	dirs := DetectToolDirs(home)
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d: %v", len(dirs), dirs)
	}
}

func TestDetectToolDirs_FindsMultiple(t *testing.T) {
	home := t.TempDir()
	for _, rel := range []string{".claude/skills", ".cursor/skills"} {
		if err := os.MkdirAll(filepath.Join(home, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dirs := DetectToolDirs(home)
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d: %v", len(dirs), dirs)
	}
}
