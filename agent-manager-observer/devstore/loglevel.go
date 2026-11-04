// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build devstore

package devstore

import (
	"regexp"
	"strings"
)

// declaredLevelRe captures the value of a level/severity field in logfmt or JSON,
// e.g. level=info or "severity":"ERROR".
var declaredLevelRe = regexp.MustCompile(
	`(?i)(?:^|[\s,{])"?(?:lvl|level|severity_?text|severity|levelname|@l)"?\s*[=:]\s*"?([a-z]+)`)

// klogHeaderRe matches the single-letter header some loggers emit, e.g. "E0501 12:34:56 ...";
// the time component guards against matching arbitrary "F1234 ..." text.
var klogHeaderRe = regexp.MustCompile(`^([IWEF])\d{4} \d{2}:\d{2}:\d{2}`)

// levelMatchers extract a level from a known shape, tried in order: a declared
// level/severity field first, then the single-letter header.
var levelMatchers = []struct {
	re   *regexp.Regexp
	norm func(token string) string
}{
	{declaredLevelRe, normalizeLogLevel},
	{klogHeaderRe, normalizeKlogPrefix},
}

// fallbackLevels scan for a bare severity word, used only when no format matched. A word
// followed by '=' is a field key (e.g. error="<nil>" or error = "<nil>"), not a severity,
// so it is skipped.
var fallbackLevels = []struct {
	name string
	re   *regexp.Regexp
}{
	{"ERROR", regexp.MustCompile(`\bERROR\b(?:\s*[^=\s]|\s*$)`)},
	{"FATAL", regexp.MustCompile(`\bFATAL\b(?:\s*[^=\s]|\s*$)`)},
	{"SEVERE", regexp.MustCompile(`\bSEVERE\b(?:\s*[^=\s]|\s*$)`)},
	{"WARN", regexp.MustCompile(`\bWARNING\b(?:\s*[^=\s]|\s*$)`)},
	{"WARN", regexp.MustCompile(`\bWARN\b(?:\s*[^=\s]|\s*$)`)},
	{"INFO", regexp.MustCompile(`\bINFO\b(?:\s*[^=\s]|\s*$)`)},
	{"DEBUG", regexp.MustCompile(`\bDEBUG\b(?:\s*[^=\s]|\s*$)`)},
}

// extractLogLevel returns the level of a raw log line, trusting a declared level when
// present and otherwise scanning for a severity keyword. Defaults to INFO.
func extractLogLevel(log string) string {
	for _, m := range levelMatchers {
		if sub := m.re.FindStringSubmatch(log); sub != nil {
			if level := m.norm(sub[1]); level != "" {
				return level
			}
		}
	}

	upper := strings.ToUpper(log)
	for _, lvl := range fallbackLevels {
		if lvl.re.MatchString(upper) {
			return lvl.name
		}
	}

	return "INFO"
}

// normalizeKlogPrefix maps a klog/glog severity letter to a level.
func normalizeKlogPrefix(letter string) string {
	switch letter {
	case "I":
		return "INFO"
	case "W":
		return "WARN"
	case "E":
		return "ERROR"
	case "F":
		return "FATAL"
	}
	return ""
}

// normalizeLogLevel maps a declared level token to a canonical level; unknown values
// default to INFO.
func normalizeLogLevel(v string) string {
	switch strings.ToUpper(v) {
	case "ERROR", "ERR":
		return "ERROR"
	case "FATAL", "CRITICAL", "CRIT", "PANIC", "DPANIC", "EMERG", "EMERGENCY", "ALERT":
		return "FATAL"
	case "SEVERE":
		return "SEVERE"
	case "WARN", "WARNING":
		return "WARN"
	case "INFO", "INFORMATION", "NOTICE", "CONFIG":
		return "INFO"
	case "DEBUG", "TRACE", "VERBOSE", "FINE", "FINER", "FINEST", "SILLY":
		return "DEBUG"
	default:
		return "INFO"
	}
}
