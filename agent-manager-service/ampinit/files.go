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

package ampinit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// owner is a uid:gid the written files are handed to; nil leaves them as created.
type owner struct{ uid, gid int }

// parseOwner reads "uid:gid" from the environment variable key.
func parseOwner(key string) (*owner, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil, nil //nolint:nilnil // unset means "do not chown"
	}
	u, g, ok := strings.Cut(v, ":")
	uid, err1 := strconv.Atoi(u)
	gid, err2 := strconv.Atoi(g)
	if !ok || err1 != nil || err2 != nil {
		return nil, fmt.Errorf("%s: %q is not uid:gid", key, v)
	}
	return &owner{uid: uid, gid: gid}, nil
}

// writeFile replaces path with data unless it already holds exactly that, then applies mode and owner.
func writeFile(path string, data []byte, mode os.FileMode, o *owner) error {
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
	return settle(path, mode, o)
}

// writeOnce creates path with what gen returns unless it already exists, and returns its content.
func writeOnce(path string, mode os.FileMode, o *owner, gen func() ([]byte, error)) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return data, settle(path, mode, o)
	}
	data, err := gen()
	if err != nil {
		return nil, err
	}
	return data, writeFile(path, data, mode, o)
}

func settle(path string, mode os.FileMode, o *owner) error {
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	if o != nil {
		return os.Lchown(path, o.uid, o.gid)
	}
	return nil
}

// chownDir hands a directory the files are written into to their owner.
func chownDir(dir string, o *owner) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if o != nil {
		return os.Chown(dir, o.uid, o.gid)
	}
	return nil
}
