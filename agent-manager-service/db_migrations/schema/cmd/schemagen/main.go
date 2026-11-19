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

// Command schemagen renders db_migrations/schema/lite.yaml into the per-database migrations (the baseline and each
// numbered migration after it), and each module's schema files (modules/<dir>/schema/*.yaml) into that module's
// per-database migrations. Run it from the
// module root (`make schema`).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "schemagen:", err)
		os.Exit(1)
	}
}

func run() error {
	s, err := schema.Lite()
	if err != nil {
		return err
	}
	for _, dialect := range schema.Dialects {
		for _, m := range s.Files() {
			ddl, err := s.RenderMigration(dialect, m.ID)
			if err != nil {
				return err
			}
			if err := write(schema.MigrationPath(dialect, m), ddl); err != nil {
				return err
			}
		}
	}
	modules, err := schema.ModuleSchemas(".")
	if err != nil {
		return err
	}
	for _, m := range modules {
		if err := renderModule(m); err != nil {
			return err
		}
	}
	return nil
}

func renderModule(m schema.ModuleSchema) error {
	name, err := m.Module()
	if err != nil {
		return err
	}
	migrations, err := schema.RenderModule(name, m.Files)
	if err != nil {
		return err
	}
	for _, dialect := range schema.Dialects {
		keep := map[string]bool{}
		for _, mig := range migrations {
			path := schema.ModuleMigrationPath(m.Dir, dialect, mig.Version)
			keep[filepath.Base(path)] = true
			if err := write(path, mig.SQL[dialect]); err != nil {
				return err
			}
		}
		// Drop rendered files whose schema file is gone.
		stale, err := filepath.Glob(filepath.Join(filepath.Dir(schema.ModuleMigrationPath(m.Dir, dialect, "x")), "*.sql"))
		if err != nil {
			return err
		}
		for _, f := range stale {
			if !keep[filepath.Base(f)] {
				if err := os.Remove(f); err != nil {
					return err
				}
				fmt.Println("removed", f)
			}
		}
	}
	return nil
}

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}
