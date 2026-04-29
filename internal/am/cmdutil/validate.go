package cmdutil

import "strings"

// ValidatePathParam checks that value is safe to embed in a URL path segment.
// label describes the parameter for error messages (e.g. "agent name").
func ValidatePathParam(label, value string) error {
	if strings.TrimSpace(value) == "" {
		return FlagErrorf("%s must not be empty", label)
	}
	if strings.Contains(value, "/") {
		return FlagErrorf("%s must not contain '/'", label)
	}
	return nil
}
