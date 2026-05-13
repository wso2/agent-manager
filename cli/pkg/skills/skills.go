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
	"fmt"
	"io/fs"
	"os"
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

// DefaultDestRel is the relative path (from home) for the canonical skill directory.
const DefaultDestRel = ".agents/skills"

// knownToolDirs are the relative paths (from home) of tool skill directories.
var knownToolDirs = []string{
	filepath.Join(".claude", "skills"),
	filepath.Join(".cursor", "skills"),
	filepath.Join(".windsurf", "skills"),
}

// EmbeddedSkills returns the names of all skills bundled in the binary.
func EmbeddedSkills() []string {
	entries, err := fs.ReadDir(embedded, "skilldata")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
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

// Install extracts all embedded skills to destDir and creates symlinks
// from each toolDir to the canonical skill directory.
func Install(destDir string, toolDirs []string) (InstallResult, error) {
	var result InstallResult

	names := EmbeddedSkills()
	for _, name := range names {
		skillDir := filepath.Join(destDir, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return result, fmt.Errorf("create skill dir %s: %w", skillDir, err)
		}
		if err := extractSkill(name, skillDir); err != nil {
			return result, fmt.Errorf("extract %s: %w", name, err)
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

func extractSkill(name, destDir string) error {
	srcDir := "skilldata/" + name
	entries, err := fs.ReadDir(embedded, srcDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(embedded, srcDir+"/"+e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destDir, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
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
