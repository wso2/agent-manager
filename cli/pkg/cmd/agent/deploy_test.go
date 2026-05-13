// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
// Licensed under the Apache License, Version 2.0.

package agent

import (
	"sort"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func boolPtr(b bool) *bool { return &b }

func TestParseEnvFlag(t *testing.T) {
	cases := []struct {
		name    string
		inputs  []string
		want    map[string]string
		wantErr string
	}{
		{"single pair", []string{"A=1"}, map[string]string{"A": "1"}, ""},
		{"value with equals", []string{"URL=k=v"}, map[string]string{"URL": "k=v"}, ""},
		{"empty value", []string{"A="}, map[string]string{"A": ""}, ""},
		{"multiple", []string{"A=1", "B=2"}, map[string]string{"A": "1", "B": "2"}, ""},
		{"duplicate last-wins", []string{"A=1", "A=2"}, map[string]string{"A": "2"}, ""},
		{"empty key", []string{"=foo"}, nil, `invalid --env "=foo": empty key`},
		{"no equals", []string{"FOO"}, nil, `invalid --env "FOO": expected KEY=VALUE`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnvFlag(tc.inputs)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("got[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestFindLowestEnvironment(t *testing.T) {
	cases := []struct {
		name  string
		paths []gen.PromotionPath
		want  string
	}{
		{
			name: "linear dev->staging->prod, dev is entry",
			paths: []gen.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "staging"}}},
				{SourceEnvironmentRef: "staging", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "prod"}}},
			},
			want: "dev",
		},
		{
			name:  "empty pipeline",
			paths: nil,
			want:  "",
		},
		{
			name: "single path dev->prod",
			paths: []gen.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "prod"}}},
			},
			want: "dev",
		},
		{
			name: "every source is also a target (cycle) -> empty",
			paths: []gen.PromotionPath{
				{SourceEnvironmentRef: "a", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "b"}}},
				{SourceEnvironmentRef: "b", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "a"}}},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findLowestEnvironment(tc.paths)
			if got != tc.want {
				t.Errorf("findLowestEnvironment = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMergeEnv(t *testing.T) {
	type result struct {
		final     map[string]string
		conflicts []string
	}
	cases := []struct {
		name    string
		current []gen.ConfigurationItem
		cli     map[string]string
		want    result
	}{
		{
			name:    "no current, no cli",
			current: nil, cli: nil,
			want: result{final: map[string]string{}, conflicts: nil},
		},
		{
			name: "preserve current when cli absent",
			current: []gen.ConfigurationItem{
				{Key: "A", Value: "1"},
				{Key: "B", Value: "2"},
			},
			cli:  nil,
			want: result{final: map[string]string{"A": "1", "B": "2"}, conflicts: nil},
		},
		{
			name:    "add new cli key",
			current: []gen.ConfigurationItem{{Key: "A", Value: "1"}},
			cli:     map[string]string{"B": "2"},
			want:    result{final: map[string]string{"A": "1", "B": "2"}, conflicts: nil},
		},
		{
			name:    "same value is not a conflict",
			current: []gen.ConfigurationItem{{Key: "A", Value: "1"}},
			cli:     map[string]string{"A": "1"},
			want:    result{final: map[string]string{"A": "1"}, conflicts: nil},
		},
		{
			name:    "different value is a conflict",
			current: []gen.ConfigurationItem{{Key: "A", Value: "1"}},
			cli:     map[string]string{"A": "2"},
			want:    result{final: map[string]string{"A": "2"}, conflicts: []string{"A"}},
		},
		{
			name: "sensitive current key always conflicts when cli sets it",
			current: []gen.ConfigurationItem{
				{Key: "SECRET", Value: "", IsSensitive: boolPtr(true)},
			},
			cli:  map[string]string{"SECRET": "new"},
			want: result{final: map[string]string{"SECRET": "new"}, conflicts: []string{"SECRET"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			final, conflicts := mergeEnv(tc.current, tc.cli)

			gotFinal := map[string]string{}
			for _, ev := range final {
				if ev.Value == nil {
					gotFinal[ev.Key] = ""
				} else {
					gotFinal[ev.Key] = *ev.Value
				}
			}
			if len(gotFinal) != len(tc.want.final) {
				t.Errorf("final size = %d, want %d (%v vs %v)", len(gotFinal), len(tc.want.final), gotFinal, tc.want.final)
			}
			for k, v := range tc.want.final {
				if gotFinal[k] != v {
					t.Errorf("final[%q] = %q, want %q", k, gotFinal[k], v)
				}
			}

			gotConflicts := make([]string, 0, len(conflicts))
			for _, c := range conflicts {
				gotConflicts = append(gotConflicts, c.Key)
			}
			sort.Strings(gotConflicts)
			wantConflicts := append([]string{}, tc.want.conflicts...)
			sort.Strings(wantConflicts)
			if len(gotConflicts) != len(wantConflicts) {
				t.Fatalf("conflicts = %v, want %v", gotConflicts, wantConflicts)
			}
			for i := range gotConflicts {
				if gotConflicts[i] != wantConflicts[i] {
					t.Errorf("conflicts[%d] = %q, want %q", i, gotConflicts[i], wantConflicts[i])
				}
			}
		})
	}
}

func TestRenderConflictTable_PlainOnly(t *testing.T) {
	io, _, _, errOut := iostreams.Test()
	io.SetTerminal(true, true, true)
	conflicts := []envConflict{
		{Key: "OPENAI_MODEL", CurrentValue: "gpt-4o-mini", NewValue: "gpt-4o", CurrentSensitive: false},
	}
	renderConflictTable(io, conflicts)
	out := errOut.String()
	if strings.Contains(out, "secret") {
		t.Errorf("plain-only render should not mention secrets, got: %q", out)
	}
	for _, want := range []string{"OPENAI_MODEL", "gpt-4o-mini", "gpt-4o"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestRenderConflictTable_SensitiveBanner(t *testing.T) {
	io, _, _, errOut := iostreams.Test()
	io.SetTerminal(true, true, true)
	conflicts := []envConflict{
		{Key: "PINECONE_API_KEY", CurrentValue: "", NewValue: "new-secret-value", CurrentSensitive: true},
	}
	renderConflictTable(io, conflicts)
	out := errOut.String()
	if !strings.Contains(out, "PINECONE_API_KEY") {
		t.Errorf("banner should name affected key, got: %q", out)
	}
	if !strings.Contains(out, "secret") {
		t.Errorf("banner should mention secret demotion, got: %q", out)
	}
	if !strings.Contains(out, "(secret)") {
		t.Errorf("table should render current sensitive value as (secret), got: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("table should render incoming sensitive value as ***, got: %q", out)
	}
	if strings.Contains(out, "new-secret-value") {
		t.Errorf("table must NOT echo incoming CLI value for sensitive key, got: %q", out)
	}
}
