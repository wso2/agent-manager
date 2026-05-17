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
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillMeta holds metadata parsed from SKILL.md frontmatter.
type SkillMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// InstalledSkill describes a skill on disk.
type InstalledSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// LinkInfo describes a symlink created in a tool directory.
type LinkInfo struct {
	Skill    string `json:"skill"`
	LinkPath string `json:"link_path"`
}

// InstallResult holds the outcome of an Install operation.
type InstallResult struct {
	Skills []InstalledSkill `json:"skills"`
	Links  []LinkInfo       `json:"links"`
}

// RemoveResult holds the outcome of a Remove operation.
type RemoveResult struct {
	RemovedSkills []string `json:"removed_skills"`
	RemovedLinks  []string `json:"removed_links"`
}

// SkillInfo describes a skill in the catalog and any on-disk presence.
// Path is omitted from JSON when the skill exists only on the remote.
type SkillInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Path        string   `json:"path,omitempty"`
	ActiveLinks []string `json:"active_links,omitempty"`
	NativeTools []string `json:"native_tools,omitempty"`
}

// DefaultDestRel is the relative path (from home) for the canonical skill directory.
const DefaultDestRel = ".agents/skills"

// KnownNativeTools lists tools that read installed skills directly from
// DefaultDestRel and therefore do not need a per-tool symlink directory.
// Codex discovers skills from $HOME/.agents/skills natively
// (https://developers.openai.com/codex/skills.md).
var KnownNativeTools = []string{"codex"}

// validSkillName reports whether name is safe to use as a top-level skill
// directory: non-empty, not "." or "..", and contains no path separators.
func validSkillName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// validSkillRelPath reports whether rel is safe to use as a relative path
// inside a skill directory: non-empty, not absolute, no backslashes (which
// can act as separators on some platforms), and no ".." segments after
// path.Clean.
func validSkillRelPath(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsRune(rel, '\\') {
		return false
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// knownToolDirs are the relative paths (from home) of tool skill directories.
var knownToolDirs = []string{
	filepath.Join(".claude", "skills"),
	filepath.Join(".cursor", "skills"),
	filepath.Join(".windsurf", "skills"),
}

// DetectToolDirs returns the subset of known tool skill directories that
// exist under homeDir.
func DetectToolDirs(homeDir string) []string {
	var dirs []string
	for _, rel := range knownToolDirs {
		abs := filepath.Join(homeDir, rel)
		info, err := os.Stat(abs)
		if err == nil && info.IsDir() {
			dirs = append(dirs, abs)
		}
	}
	return dirs
}

// Install reads every skill under fsys's "skilldata" root, writes it to
// destDir, and creates symlinks from each toolDir to the canonical skill
// directory.
func Install(ctx context.Context, fsys fs.FS, destDir string, toolDirs []string) (InstallResult, error) {
	var result InstallResult

	entries, err := fs.ReadDir(fsys, "skilldata")
	if err != nil {
		return result, fmt.Errorf("read skilldata: %w", err)
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !validSkillName(name) {
			return result, fmt.Errorf("invalid skill name %q", name)
		}
		skillDir := filepath.Join(destDir, name)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return result, fmt.Errorf("create dest dir %s: %w", destDir, err)
		}
		tmpDir := skillDir + ".tmp"
		// Clean any leftover tmp from a prior aborted install.
		if err := os.RemoveAll(tmpDir); err != nil {
			return result, fmt.Errorf("clear stale tmp %s: %w", tmpDir, err)
		}
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			return result, fmt.Errorf("create tmp dir %s: %w", tmpDir, err)
		}
		if err := extractSkillFS(fsys, name, tmpDir); err != nil {
			os.RemoveAll(tmpDir)
			return result, fmt.Errorf("extract %s: %w", name, err)
		}
		if err := os.RemoveAll(skillDir); err != nil {
			os.RemoveAll(tmpDir)
			return result, fmt.Errorf("remove old skill dir %s: %w", skillDir, err)
		}
		if err := os.Rename(tmpDir, skillDir); err != nil {
			os.RemoveAll(tmpDir)
			return result, fmt.Errorf("install %s: %w", skillDir, err)
		}
		meta := readMeta(filepath.Join(skillDir, "SKILL.md"))
		result.Skills = append(result.Skills, InstalledSkill{
			Name:        name,
			Description: meta.Description,
			Path:        skillDir,
		})

		for _, td := range toolDirs {
			linkPath := filepath.Join(td, name)
			if err := removeIfSymlink(linkPath); err != nil {
				return result, fmt.Errorf("clear existing %s: %w", linkPath, err)
			}
			if err := os.Symlink(skillDir, linkPath); err != nil {
				return result, fmt.Errorf("symlink %s → %s: %w", linkPath, skillDir, err)
			}
			result.Links = append(result.Links, LinkInfo{
				Skill:    name,
				LinkPath: linkPath,
			})
		}
	}
	return result, nil
}

// List enumerates the catalog from fsys (the remote source) and overlays
// installed/linked status from destDir and toolDirs.
func List(ctx context.Context, fsys fs.FS, destDir string, toolDirs []string) ([]SkillInfo, error) {
	entries, err := fs.ReadDir(fsys, "skilldata")
	if err != nil {
		return nil, fmt.Errorf("read skilldata: %w", err)
	}

	var infos []SkillInfo
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, _ := fs.ReadFile(fsys, "skilldata/"+name+"/SKILL.md")
		meta := parseFrontmatter(data)

		info := SkillInfo{
			Name:        name,
			Description: meta.Description,
		}

		skillDir := filepath.Join(destDir, name)
		if _, err := os.Stat(skillDir); err == nil {
			info.Path = skillDir
			info.NativeTools = append(info.NativeTools, KnownNativeTools...)
		}

		for _, td := range toolDirs {
			linkPath := filepath.Join(td, name)
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}
			if target == skillDir {
				info.ActiveLinks = append(info.ActiveLinks, linkPath)
			}
		}

		infos = append(infos, info)
	}
	return infos, nil
}

// Remove walks destDir for installed skills (subdirectories containing a
// SKILL.md), scrubs any symlinks in toolDirs that point at them, and
// deletes the canonical directories. Disk-only — no network access.
func Remove(destDir string, toolDirs []string) (RemoveResult, error) {
	var result RemoveResult

	entries, err := os.ReadDir(destDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("read dest dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		skillDir := filepath.Join(destDir, name)
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			continue
		}

		for _, td := range toolDirs {
			linkPath := filepath.Join(td, name)
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(target, destDir+string(filepath.Separator)) {
				continue
			}
			if err := os.Remove(linkPath); err != nil {
				return result, fmt.Errorf("remove symlink %s: %w", linkPath, err)
			}
			result.RemovedLinks = append(result.RemovedLinks, linkPath)
		}

		if err := os.RemoveAll(skillDir); err != nil {
			return result, fmt.Errorf("remove skill dir %s: %w", skillDir, err)
		}
		result.RemovedSkills = append(result.RemovedSkills, name)
	}
	return result, nil
}

// extractSkillFS copies every regular file under skilldata/<name> in fsys
// to destDir, preserving subdirectory structure.
func extractSkillFS(fsys fs.FS, name, destDir string) error {
	root := "skilldata/" + name
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if !validSkillRelPath(filepath.ToSlash(rel)) {
			return fmt.Errorf("invalid skill file path %q", rel)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out := filepath.Join(destDir, rel)
		cleanedDest := filepath.Clean(destDir) + string(filepath.Separator)
		if !strings.HasPrefix(filepath.Clean(out), cleanedDest) {
			return fmt.Errorf("skill file %q escapes destination", rel)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

func removeIfSymlink(path string) error {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	return fmt.Errorf("%s exists and is not a symlink; remove it manually", path)
}

func readMeta(path string) SkillMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillMeta{}
	}
	return parseFrontmatter(data)
}

func parseFrontmatter(data []byte) SkillMeta {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return SkillMeta{}
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return SkillMeta{}
	}
	var meta SkillMeta
	if err := yaml.Unmarshal([]byte(content[4:4+end]), &meta); err != nil {
		return SkillMeta{}
	}
	return meta
}
