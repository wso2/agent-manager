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

package dbx_test

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var updatePortability = flag.Bool("update-portability", false, "rewrite testdata/portability_baseline.txt from the code")

// postgresOnly matches SQL and GORM clauses that do not run unchanged on every database (schema rule 10).
var postgresOnly = regexp.MustCompile(strings.Join([]string{
	`\bNOW\(\)`, `CURRENT_TIMESTAMP`, `INTERVAL\b`, `::[a-z]`, `ON CONFLICT`, `\bRETURNING\b`, `FOR UPDATE`,
	`SKIP LOCKED`, `FOR SHARE`, `ILIKE`, `DISTINCT ON`, `\bpg_[a-z_]+\(`, `jsonb`, `->>`, `@>`, `gen_random_uuid`,
	`clause\.OnConflict`, `clause\.Locking`, `clause\.Returning`, `pgconn\.`, `GREATEST\(`, `date_trunc`, `EXTRACT\(`,
}, "|"))

const portabilityBaseline = "testdata/portability_baseline.txt"

// countPostgresOnly counts matching code lines (not comments) per Go file outside tests, generated code and the
// historical PostgreSQL migrations.
func countPostgresOnly(t *testing.T, root string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch rel {
			// db/sqlitedialect is the SQLite dialect itself: it names these clauses to translate them.
			case "tmp", "spec", "vendor", "clients/clientmocks", "db/sqlitedialect":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		// The numbered historical migrations are PostgreSQL-only by design; new schema changes are SQL files.
		if dir, name := filepath.Split(rel); dir == "db_migrations/" && name[0] >= '0' && name[0] <= '9' {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(body), "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "//") {
				continue
			}
			if postgresOnly.MatchString(code) {
				counts[rel]++
			}
		}
		return nil
	})
	require.NoError(t, err)
	return counts
}

// TestNoNewPostgresOnlySQL is a ratchet: a file may not gain PostgreSQL-only SQL, and a file that loses some must
// lower its recorded count (run with -update-portability) so it cannot come back.
func TestNoNewPostgresOnlySQL(t *testing.T) {
	got := countPostgresOnly(t, filepath.Join("..", ".."))

	if *updatePortability {
		var lines []string
		for f, n := range got {
			lines = append(lines, fmt.Sprintf("%s %d", f, n))
		}
		sort.Strings(lines)
		body := "# PostgreSQL-only lines per file (schema rule 10). Only ever lowered: go test ./db/dbx -update-portability\n" +
			strings.Join(lines, "\n") + "\n"
		require.NoError(t, os.WriteFile(portabilityBaseline, []byte(body), 0o600))
		return
	}

	want := map[string]int{}
	data, err := os.ReadFile(portabilityBaseline)
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		file, n, _ := strings.Cut(line, " ")
		want[file], err = strconv.Atoi(n)
		require.NoError(t, err, line)
	}

	for f, n := range got {
		if n > want[f] {
			t.Errorf("%s: %d PostgreSQL-only lines, %d allowed. Use the portable patterns in db/dbx (schema rule 10).", f, n, want[f])
		}
	}
	for f, n := range want {
		if got[f] < n {
			t.Errorf("%s: down to %d PostgreSQL-only lines from %d; lower the record: go test ./db/dbx -update-portability", f, got[f], n)
		}
	}
}
