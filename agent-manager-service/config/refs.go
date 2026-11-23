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
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// refPattern is a reference that is the whole value: ${env:NAME} or
// ${file:/path}, optionally with ":-default".
var refPattern = regexp.MustCompile(`^\$\{(env|file):([^}:]+)(?::-(.*))?\}$`)

// escapedRef is a value starting with "$${" (or more "$"): one "$" is dropped.
var escapedRef = regexp.MustCompile(`^\$\$+\{`)

// resolveRef returns raw with a reference replaced by what it names. Anything
// else is returned as is, except an escaped "$${", which stands for "${".
func resolveRef(key, raw string, getenv func(string) (string, bool), readFile func(string) ([]byte, error)) (string, error) {
	v := strings.TrimSpace(raw)
	if escapedRef.MatchString(v) {
		return raw[strings.Index(raw, "$")+1:], nil
	}
	m := refPattern.FindStringSubmatch(v)
	if m == nil {
		return raw, nil
	}
	scheme, name, fallback := m[1], m[2], m[3]
	hasFallback := strings.Contains(v, name+":-")
	switch scheme {
	case "env":
		if val, ok := getenv(name); ok && val != "" {
			return val, nil
		}
		if hasFallback {
			return fallback, nil
		}
		return "", fmt.Errorf("%s: environment variable %s is not set", envName(key), name)
	default:
		data, err := readFile(name)
		if err == nil {
			return trimNewline(string(data)), nil
		}
		if hasFallback && errors.Is(err, fs.ErrNotExist) {
			return fallback, nil
		}
		return "", fmt.Errorf("%s: reading %s: %w", envName(key), name, err)
	}
}

// trimNewline drops the line ending an editor or `echo` leaves on a file.
func trimNewline(s string) string {
	return strings.TrimRight(s, "\r\n")
}
