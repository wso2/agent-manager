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
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

const skillFrontmatter = "---\nname: use-amctl\ndescription: test description\n---\n\nbody"

func fakeFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"skilldata/use-amctl/SKILL.md": &fstest.MapFile{
			Data: []byte(skillFrontmatter),
			Mode: 0o644,
		},
		"skilldata/use-amctl/references/extra.md": &fstest.MapFile{
			Data: []byte("extra content"),
			Mode: 0o644,
		},
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

func TestInstall_ExtractsAndLinks(t *testing.T) {
	dest := t.TempDir()
	toolDir := t.TempDir()

	result, err := Install(context.Background(), fakeFS(t), dest, []string{toolDir})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "use-amctl" {
		t.Errorf("skill name = %q, want use-amctl", result.Skills[0].Name)
	}

	skillMD := filepath.Join(dest, "use-amctl", "SKILL.md")
	if _, err := os.Stat(skillMD); err != nil {
		t.Errorf("SKILL.md not found at %s: %v", skillMD, err)
	}
	nested := filepath.Join(dest, "use-amctl", "references", "extra.md")
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("nested file not found at %s: %v", nested, err)
	}

	linkPath := filepath.Join(toolDir, "use-amctl")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", linkPath, err)
	}
	expectedTarget := filepath.Join(dest, "use-amctl")
	if target != expectedTarget {
		t.Errorf("symlink target = %q, want %q", target, expectedTarget)
	}

	if len(result.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(result.Links))
	}
}

func TestInstall_Idempotent(t *testing.T) {
	dest := t.TempDir()
	toolDir := t.TempDir()

	if _, err := Install(context.Background(), fakeFS(t), dest, []string{toolDir}); err != nil {
		t.Fatalf("first Install failed: %v", err)
	}
	result, err := Install(context.Background(), fakeFS(t), dest, []string{toolDir})
	if err != nil {
		t.Fatalf("second Install failed: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill after second install, got %d", len(result.Skills))
	}
}

func TestInstall_NoToolDirs(t *testing.T) {
	dest := t.TempDir()

	result, err := Install(context.Background(), fakeFS(t), dest, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if len(result.Links) != 0 {
		t.Errorf("expected 0 links, got %d", len(result.Links))
	}
}

func TestRemove_CleansUpSymlinksAndDirs(t *testing.T) {
	dest := t.TempDir()
	toolDir := t.TempDir()

	if _, err := Install(context.Background(), fakeFS(t), dest, []string{toolDir}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	result, err := Remove(dest, []string{toolDir})
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(result.RemovedSkills) != 1 || result.RemovedSkills[0] != "use-amctl" {
		t.Errorf("removed skills = %v, want [use-amctl]", result.RemovedSkills)
	}
	if len(result.RemovedLinks) != 1 {
		t.Errorf("removed links = %v, want 1 entry", result.RemovedLinks)
	}

	if _, err := os.Stat(filepath.Join(dest, "use-amctl")); !os.IsNotExist(err) {
		t.Error("canonical dir should be removed")
	}
	if _, err := os.Lstat(filepath.Join(toolDir, "use-amctl")); !os.IsNotExist(err) {
		t.Error("symlink should be removed")
	}
}

func TestRemove_NothingInstalled(t *testing.T) {
	dest := t.TempDir()
	toolDir := t.TempDir()

	result, err := Remove(dest, []string{toolDir})
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(result.RemovedSkills) != 0 {
		t.Errorf("expected 0 removed skills, got %d", len(result.RemovedSkills))
	}
}

func TestRemove_DestDirDoesNotExist(t *testing.T) {
	// dest path that doesn't exist at all; Remove should no-op cleanly.
	dest := filepath.Join(t.TempDir(), "does-not-exist")
	result, err := Remove(dest, nil)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(result.RemovedSkills) != 0 {
		t.Errorf("expected 0 removed skills, got %d", len(result.RemovedSkills))
	}
}

func TestList_BeforeAndAfterInstall(t *testing.T) {
	dest := t.TempDir()
	toolDir := t.TempDir()
	fsys := fakeFS(t)

	infos, err := List(context.Background(), fsys, dest, []string{toolDir})
	if err != nil {
		t.Fatalf("List before install failed: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 remote skill, got %d", len(infos))
	}
	if infos[0].Name != "use-amctl" {
		t.Errorf("name = %q, want use-amctl", infos[0].Name)
	}
	if infos[0].Description != "test description" {
		t.Errorf("description = %q, want %q", infos[0].Description, "test description")
	}
	if infos[0].Path != "" {
		t.Errorf("Path should be empty before install, got %q", infos[0].Path)
	}
	if len(infos[0].ActiveLinks) != 0 {
		t.Errorf("ActiveLinks should be empty before install, got %v", infos[0].ActiveLinks)
	}

	if _, err := Install(context.Background(), fsys, dest, []string{toolDir}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	infos, err = List(context.Background(), fsys, dest, []string{toolDir})
	if err != nil {
		t.Fatalf("List after install failed: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(infos))
	}
	wantPath := filepath.Join(dest, "use-amctl")
	if infos[0].Path != wantPath {
		t.Errorf("Path = %q, want %q", infos[0].Path, wantPath)
	}
	if len(infos[0].ActiveLinks) != 1 {
		t.Errorf("expected 1 active link, got %d", len(infos[0].ActiveLinks))
	}
}

func TestRemove_SkipsNonAmctlSymlinks(t *testing.T) {
	dest := t.TempDir()
	toolDir := t.TempDir()

	// Pre-populate destDir with a non-skills directory so Remove finds nothing
	// matching its discovery rule (no SKILL.md inside).
	if err := os.MkdirAll(filepath.Join(dest, "scratch"), 0o755); err != nil {
		t.Fatal(err)
	}

	// User-created content in the tool dir (real directory, not a symlink).
	userDir := filepath.Join(toolDir, "use-amctl")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "SKILL.md"), []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Remove(dest, []string{toolDir})
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(result.RemovedLinks) != 0 {
		t.Errorf("should not remove user content, got removed links: %v", result.RemovedLinks)
	}
	if _, err := os.Stat(filepath.Join(userDir, "SKILL.md")); err != nil {
		t.Error("user content should still exist")
	}
}
