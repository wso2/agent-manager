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
	"io/fs"
	"os"
	"path/filepath"
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
