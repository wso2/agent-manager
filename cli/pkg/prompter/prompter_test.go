// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
// Licensed under the Apache License, Version 2.0.

package prompter

import (
	"bytes"
	"strings"
	"testing"
)

func TestLinePrompter_Confirm(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"yes lowercase", "y\n", true},
		{"yes word", "yes\n", true},
		{"YES uppercase", "YES\n", true},
		{"yes with whitespace", "  y  \n", true},
		{"no lowercase", "n\n", false},
		{"no word", "no\n", false},
		{"empty defaults to no", "\n", false},
		{"junk defaults to no", "maybe\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			p := New(strings.NewReader(tc.input), out)
			got, err := p.Confirm("Proceed?")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("Confirm() = %v, want %v", got, tc.want)
			}
			if !strings.Contains(out.String(), "Proceed?") {
				t.Errorf("prompt not written to out: %q", out.String())
			}
		})
	}
}

func TestLinePrompter_ConfirmDeletion_StillWorks(t *testing.T) {
	out := &bytes.Buffer{}
	p := New(strings.NewReader("foo\n"), out)
	if err := p.ConfirmDeletion("foo"); err != nil {
		t.Fatalf("ConfirmDeletion: %v", err)
	}
}
