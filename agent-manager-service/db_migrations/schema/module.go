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

package schema

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Module schemas live beside each module's code, under the service's modules directory:
//
//	modules/<dir>/schema/NNN_description.yaml           written by hand, one file per migration
//	modules/<dir>/migrations/<dialect>/NNN_description.sql  rendered by `make schema`
//
// A module embeds its migrations directory and returns it from Migrations(). Each file names its module and may
// only create tables named mod_<module>_<table>; it may reference tables from the module's earlier files.

// ModulesDir is the directory holding the modules, relative to the service's module root.
const ModulesDir = "modules"

// TestModulesDir holds modules that only tests link. Their schemas are
// rendered the same way, so the tests run real module migrations.
const TestModulesDir = "tests/modules"

// modulePrefix starts every module table name. Core tables may not use it.
const modulePrefix = "mod_"

var moduleFilePattern = regexp.MustCompile(`^[0-9]{3}_[a-z0-9_]+\.yaml$`)

// ModuleTablePrefix is the prefix every table of a module carries: mod_<module>_, with "-" written as "_".
func ModuleTablePrefix(module string) string {
	return modulePrefix + strings.ReplaceAll(module, "-", "_") + "_"
}

// ModuleFile is one schema file of a module.
type ModuleFile struct {
	// Path is the file's path relative to the service's module root; it is named in the rendered header.
	Path string
	Data []byte
}

// ModuleMigration is one module schema file rendered for every dialect.
type ModuleMigration struct {
	// Version is the file name without its extension, e.g. 001_items.
	Version string
	// SQL maps each dialect to the rendered DDL.
	SQL map[string]string
}

// RenderModule validates a module's schema files in lexical order and renders each for every dialect.
func RenderModule(module string, files []ModuleFile) ([]ModuleMigration, error) {
	sorted := append([]ModuleFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return path.Base(sorted[i].Path) < path.Base(sorted[j].Path) })
	var prior []Table
	out := make([]ModuleMigration, 0, len(sorted))
	for _, f := range sorted {
		name := path.Base(f.Path)
		if !moduleFilePattern.MatchString(name) {
			return nil, fmt.Errorf("%s: module schema files are named NNN_description.yaml", f.Path)
		}
		s, err := decode(f.Data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		if s.Module != module {
			return nil, fmt.Errorf("%s: names module %q, want %q", f.Path, s.Module, module)
		}
		s.source, s.prior = f.Path, prior
		if err := s.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		m := ModuleMigration{Version: strings.TrimSuffix(name, ".yaml"), SQL: map[string]string{}}
		for _, d := range Dialects {
			if m.SQL[d], err = s.Render(d); err != nil {
				return nil, fmt.Errorf("%s: %w", f.Path, err)
			}
		}
		out = append(out, m)
		prior = append(prior, s.Tables...)
	}
	return out, nil
}

// ModuleSchema is the schema of one module directory.
type ModuleSchema struct {
	// Dir is the module's directory relative to the service's module root, e.g. modules/aigateway.
	Dir   string
	Files []ModuleFile
}

// ModuleSchemas finds every module directory under root/modules and
// root/tests/modules that has a schema directory.
func ModuleSchemas(root string) ([]ModuleSchema, error) {
	var out []ModuleSchema
	for _, parent := range []string{ModulesDir, TestModulesDir} {
		found, err := moduleSchemasIn(root, parent)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}

func moduleSchemasIn(root, parent string) ([]ModuleSchema, error) {
	entries, err := os.ReadDir(filepath.Join(root, parent))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ModuleSchema
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join(parent, e.Name())
		files, err := filepath.Glob(filepath.Join(root, dir, "schema", "*.yaml"))
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			continue
		}
		ms := ModuleSchema{Dir: dir}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			ms.Files = append(ms.Files, ModuleFile{Path: path.Join(dir, "schema", filepath.Base(f)), Data: data})
		}
		out = append(out, ms)
	}
	return out, nil
}

// Module reads the module name the schema files declare; every file must name the same module.
func (m ModuleSchema) Module() (string, error) {
	name := ""
	for _, f := range m.Files {
		s, err := decode(f.Data)
		if err != nil {
			return "", fmt.Errorf("%s: %w", f.Path, err)
		}
		if s.Module == "" {
			return "", fmt.Errorf("%s: no module name (add `module: <name>`)", f.Path)
		}
		if name != "" && s.Module != name {
			return "", fmt.Errorf("%s: names module %q, other files name %q", f.Path, s.Module, name)
		}
		name = s.Module
	}
	return name, nil
}

// ModuleMigrationPath is where a rendered module migration lives, relative to the service's module root.
func ModuleMigrationPath(dir, dialect, version string) string {
	return path.Join(dir, "migrations", dialect, version+".sql")
}
