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

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testEnv(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := vars[k]
		return v, ok
	}
}

func resolve(t *testing.T, raw string, env map[string]string) (string, error) {
	t.Helper()
	return resolveRef("DB_PASSWORD", raw, testEnv(env), os.ReadFile)
}

func TestResolveRefEnv(t *testing.T) {
	env := map[string]string{"A": "x", "EMPTY": ""}
	for raw, want := range map[string]string{
		"${env:A}":               "x",
		"${env:MISSING:-dflt}":   "dflt",
		"${env:EMPTY:-dflt}":     "dflt",
		"  ${env:A}  ":           "x",
		"${env:MISSING:-a:-b}":   "a:-b",
		"${env:MISSING:-}":       "",
		"${env:A:-unused}":       "x",
		"${env:MISSING:-p@ss w}": "p@ss w",
	} {
		got, err := resolve(t, raw, env)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
}

func TestResolveRefFile(t *testing.T) {
	dir := t.TempDir()
	crlf := filepath.Join(dir, "crlf")
	require.NoError(t, os.WriteFile(crlf, []byte("s3cret\r\n"), 0o600))
	lf := filepath.Join(dir, "lf")
	require.NoError(t, os.WriteFile(lf, []byte("s3cret\n"), 0o600))

	for raw, want := range map[string]string{
		"${file:" + crlf + "}": "s3cret",
		"${file:" + lf + "}":   "s3cret",
		"${file:" + filepath.Join(dir, "missing") + ":-dflt}": "dflt",
	} {
		got, err := resolve(t, raw, nil)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
}

func TestResolveRefLiteral(t *testing.T) {
	for raw, want := range map[string]string{
		"plain":            "plain",
		"pre${env:A}":      "pre${env:A}",
		"${env:A}post":     "${env:A}post",
		"${foo:bar}":       "${foo:bar}",
		"${A}":             "${A}",
		"$${env:A}":        "${env:A}",
		"$$${env:A}":       "$${env:A}",
		"p${ss}word":       "p${ss}word",
		"":                 "",
		"${env:}":          "${env:}",
		"${file:}":         "${file:}",
		"${env:A}${env:B}": "${env:A}${env:B}",
	} {
		got, err := resolve(t, raw, map[string]string{"A": "x", "B": "y"})
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
}

func TestResolveRefUnresolved(t *testing.T) {
	_, err := resolve(t, "${env:MISSING}", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PASSWORD")
	assert.Contains(t, err.Error(), "MISSING")

	_, err = resolve(t, "${file:/nope/amp-secret}", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PASSWORD")
	assert.Contains(t, err.Error(), "/nope/amp-secret")
}

func TestLoadResolvesFileReferences(t *testing.T) {
	t.Cleanup(func() {
		fileVars, fileModules = nil, Modules{}
		loadEnvs()
	})
	dir := t.TempDir()
	t.Setenv("ENV_FILE_PATH", "")
	t.Setenv(FileEnv, "")
	t.Setenv(ModulesEnv, "")
	t.Setenv("DB_PASSWORD", "")
	require.NoError(t, os.Unsetenv("DB_PASSWORD"))
	t.Setenv("PGPW", "pw")

	good := filepath.Join(dir, "good.yaml")
	require.NoError(t, os.WriteFile(good, []byte(
		"db: {password: \"${env:PGPW}\"}\nmodules:\n  - name: widgets\n    config: {token: \"${env:PGPW}\"}\n",
	), 0o600))
	require.NoError(t, Load(good))
	assert.Equal(t, "pw", Value("DB_PASSWORD"))
	assert.Equal(t, "pw", Value(ModuleConfigKey("widgets", "token")))

	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("db: {password: \"${env:AMP_TEST_UNSET_VAR}\"}\n"), 0o600))
	err := Load(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PASSWORD")
}

// A reference is resolved only where amp.yaml supplies the value: one the
// environment overrides may be unresolvable.
func TestOverriddenReferenceIsNotResolved(t *testing.T) {
	reloadAfter(t)
	t.Setenv("DB_PASSWORD", "override")
	require.NoError(t, loadWith(t, "db: {password: \"${env:AMP_TEST_UNSET_VAR}\"}\n"))
	assert.Equal(t, "override", Value("DB_PASSWORD"))

	require.NoError(t, os.Unsetenv("DB_PASSWORD"))
	err := loadWith(t, "db: {password: \"${env:AMP_TEST_UNSET_VAR}\"}\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PASSWORD")
}
