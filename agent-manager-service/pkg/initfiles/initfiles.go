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

// Package initfiles writes the files "amp init" generates: written only when
// they change, generated once and kept, and handed to the user and group the
// process that reads them runs as.
package initfiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Owner is the user and group a written file is given; nil leaves files as
// created.
type Owner struct{ uid, gid int }

// ParseOwner reads "uid:gid"; nil when v is blank.
func ParseOwner(v string) (*Owner, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil //nolint:nilnil // blank means "do not chown"
	}
	u, g, ok := strings.Cut(v, ":")
	uid, err1 := strconv.Atoi(u)
	gid, err2 := strconv.Atoi(g)
	if !ok || err1 != nil || err2 != nil {
		return nil, fmt.Errorf("%q is not uid:gid", v)
	}
	return &Owner{uid: uid, gid: gid}, nil
}

// WriteFile replaces path with data unless it already holds exactly that,
// then applies mode and owner.
func WriteFile(path string, data []byte, mode os.FileMode, o *Owner) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, data) {
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, mode); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	return Settle(path, mode, o)
}

// WriteOnce creates path with what gen returns unless it already exists, and
// returns its content either way.
func WriteOnce(path string, mode os.FileMode, o *Owner, gen func() ([]byte, error)) ([]byte, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil && len(data) > 0:
		return data, Settle(path, mode, o)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	if data, err = gen(); err != nil {
		return nil, err
	}
	return data, WriteFile(path, data, mode, o)
}

// Settle gives an existing file the mode and owner.
func Settle(path string, mode os.FileMode, o *Owner) error {
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	if o != nil {
		return os.Lchown(path, o.uid, o.gid)
	}
	return nil
}

// ChownDir creates the directory files are written into and gives it to
// the owner.
func ChownDir(dir string, o *Owner) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if o != nil {
		return os.Chown(dir, o.uid, o.gid)
	}
	return nil
}

// Quote is s as a double-quoted literal, valid in YAML, JavaScript and TOML.
func Quote(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
