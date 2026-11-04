// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build devstore

package devstore

import "testing"

func TestExtractLogLevel(t *testing.T) {
	cases := map[string]string{
		"ERROR boom":                  "ERROR",
		`{"level":"warn","msg":"x"}`:  "WARN",
		"level=debug msg=hi":          "DEBUG",
		"E0501 12:34:56.123 failure":  "ERROR",
		`msg="x" error="<nil>"`:       "INFO",
		"plain":                       "INFO",
		`{"severity":"CRITICAL"}`:     "FATAL",
		"2026 WARNING disk almost up": "WARN",
	}
	for line, want := range cases {
		if got := extractLogLevel(line); got != want {
			t.Errorf("extractLogLevel(%q) = %q, want %q", line, got, want)
		}
	}
}
