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

package controllers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoHandlerRejectsAnAbsentEnvironment fails on any handler that 400s an empty environment; handlers default it with EnvironmentOrDefault.
func TestNoHandlerRejectsAnAbsentEnvironment(t *testing.T) {
	// allowed exempts a deliberate rejection, with the reason; a stale entry fails the test.
	allowed := map[string]string{}
	seen := map[string]bool{}

	// Walk the whole service, so module handlers are covered too.
	fset := token.NewFileSet()
	var files []string
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "vendor", "testdata", "node_modules", "tmp", "spec", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range files {
		name, err := filepath.Rel("..", path)
		if err != nil {
			name = path
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				stmt, ok := n.(*ast.IfStmt)
				if !ok {
					return true
				}
				field, ok := emptyEnvironmentComparison(stmt.Cond)
				if !ok || !writesBadRequest(stmt.Body) {
					return true
				}
				key := name + ":" + fn.Name.Name
				seen[key] = true
				if reason, ok := allowed[key]; ok {
					t.Logf("%s: allowed rejection of an absent %s (%s)", key, field, reason)
					return true
				}
				t.Errorf("%s rejects an absent %s with 400, but every org has one environment and requests may omit it. "+
					"Use models.EnvironmentOrDefault, or record the reason in this test's allowlist.",
					key, field)
				return true
			})
		}
	}

	for key, reason := range allowed {
		if !seen[key] {
			t.Errorf("allowlist entry %q (%s) no longer matches any handler - delete it. "+
				"A stale exemption is how a real rejection gets waved through later.", key, reason)
		}
	}
}

// environmentFields names the ways a request body carries an environment.
var environmentFields = map[string]bool{
	"Environment": true, "EnvironmentName": true, "EnvironmentID": true,
	"EnvID": true, "EnvName": true, "EnvironmentId": true,
}

// emptyEnvironmentComparison reports whether cond is `something.<EnvField> == ""`
// (or a plain `envID == ""` on a variable named for an environment), and names
// the field.
func emptyEnvironmentComparison(cond ast.Expr) (string, bool) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL || !isEmptyString(bin.Y) {
		return "", false
	}
	switch lhs := bin.X.(type) {
	case *ast.SelectorExpr:
		if environmentFields[lhs.Sel.Name] {
			return lhs.Sel.Name, true
		}
	case *ast.Ident:
		// A handler that copies the field into a local first - `envID :=
		// payload.Environment` - then checks the local. Matching the name keeps
		// that shape covered.
		if environmentFields[strings.ToUpper(lhs.Name[:1])+lhs.Name[1:]] {
			return lhs.Name, true
		}
	}
	return "", false
}

func isEmptyString(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && (lit.Value == `""`)
}

// writesBadRequest reports whether the block refuses the request with a 400.
func writesBadRequest(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "StatusBadRequest" {
			found = true
		}
		return true
	})
	return found
}
