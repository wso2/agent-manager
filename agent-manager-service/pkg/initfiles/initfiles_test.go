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
package initfiles

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOwner(t *testing.T) {
	o, err := ParseOwner("  ")
	require.NoError(t, err)
	assert.Nil(t, o, "blank means do not chown")

	o, err = ParseOwner(" 1000:2000 ")
	require.NoError(t, err)
	assert.Equal(t, &Owner{uid: 1000, gid: 2000}, o)

	for _, bad := range []string{"1000", "a:1", "1:b", "1:2:3"} {
		_, err := ParseOwner(bad)
		require.ErrorContains(t, err, "is not uid:gid", bad)
	}
}

func TestWriteFileCreatesDirsAndSetsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "secret.pem")
	require.NoError(t, WriteFile(path, []byte("v1"), 0o600, nil))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(got))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	_, err = os.Stat(path + ".tmp")
	assert.True(t, errors.Is(err, os.ErrNotExist), "no temp file is left behind")

	// Same content: the file is not rewritten, but its mode is still settled.
	require.NoError(t, os.Chmod(path, 0o644))
	before, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, WriteFile(path, []byte("v1"), 0o600, nil))
	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
	assert.Equal(t, os.FileMode(0o600), after.Mode().Perm())

	require.NoError(t, WriteFile(path, []byte("v2"), 0o600, nil))
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got))
}

func TestWriteOnceKeepsExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	calls := 0
	gen := func() ([]byte, error) {
		calls++
		return []byte("generated"), nil
	}

	data, err := WriteOnce(path, 0o600, nil, gen)
	require.NoError(t, err)
	assert.Equal(t, "generated", string(data))
	data, err = WriteOnce(path, 0o600, nil, gen)
	require.NoError(t, err)
	assert.Equal(t, "generated", string(data))
	assert.Equal(t, 1, calls, "an existing file is never regenerated")

	// An empty file counts as absent.
	empty := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	data, err = WriteOnce(empty, 0o600, nil, gen)
	require.NoError(t, err)
	assert.Equal(t, "generated", string(data))

	errGen := errors.New("no entropy")
	_, err = WriteOnce(filepath.Join(t.TempDir(), "x"), 0o600, nil, func() ([]byte, error) { return nil, errGen })
	require.ErrorIs(t, err, errGen)

	// A path that cannot be read (a directory) is an error, not a reason to generate.
	_, err = WriteOnce(t.TempDir(), 0o600, nil, gen)
	require.Error(t, err)
}

func TestSettleAndChownDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	require.NoError(t, ChownDir(dir, nil))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Chowning to our own ids needs no privilege.
	self := &Owner{uid: os.Getuid(), gid: os.Getgid()}
	require.NoError(t, ChownDir(dir, self))
	path := filepath.Join(dir, "f")
	require.NoError(t, WriteFile(path, []byte("x"), 0o640, self))

	require.Error(t, Settle(filepath.Join(dir, "missing"), 0o600, nil))
}

func TestQuote(t *testing.T) {
	q, err := Quote("a \"b\"\n")
	require.NoError(t, err)
	assert.Equal(t, `"a \"b\"\n"`, q)
}
