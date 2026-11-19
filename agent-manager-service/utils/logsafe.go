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

package utils

import (
	"strings"
	"unicode"
)

// logForgingReplacer strips the characters that let an attacker fake extra log
// entries. CR/LF are the classic vector; the remaining control characters can
// corrupt log-based tooling and terminal output.
var logForgingReplacer = strings.NewReplacer("\n", "", "\r", "", "\x00", "", "\x1b", "")

// SanitizeForLog strips CR/LF and other control characters from untrusted input
// before it is written to logs, preventing log forging via injected newlines
// that could fake extra log entries or corrupt log-based tooling.
//
// Every attacker-controlled string that reaches a log line or an audit record
// must pass through this — audit records in particular carry user agents,
// request paths and resource names that originate from the caller.
func SanitizeForLog(s string) string {
	s = logForgingReplacer.Replace(s)
	return strings.Map(func(r rune) rune {
		if r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// TruncateForLog bounds an untrusted string to max runes, appending an ellipsis
// marker when it had to cut. Bounding happens after sanitisation so a caller
// cannot smuggle control characters past the limit.
func TruncateForLog(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
