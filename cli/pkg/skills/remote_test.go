// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

// buildTarball constructs a gzipped tar in memory from the given entries.
// Each entry's path is written as-is (caller controls the top-level dir).
func buildTarball(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for path, body := range entries {
		hdr := &tar.Header{
			Name:     path,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestWalkTarball_FiltersByPrefixAndGroupsBySkill(t *testing.T) {
	tarball := buildTarball(t, map[string]string{
		"agent-skills-main/README.md":                                            "ignored",
		"agent-skills-main/plugins/agent-manager/skills/foo/SKILL.md":            "foo-skill",
		"agent-skills-main/plugins/agent-manager/skills/foo/references/extra.md": "foo-extra",
		"agent-skills-main/plugins/agent-manager/skills/bar/SKILL.md":            "bar-skill",
		"agent-skills-main/plugins/other-product/skills/baz/SKILL.md":            "ignored-other-plugin",
	})

	got, err := walkTarball(tarball, "plugins/agent-manager/skills/")
	if err != nil {
		t.Fatalf("walkTarball: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("want 2 skills, got %d: %v", len(got), keysOf(got))
	}
	if string(got["foo"]["SKILL.md"]) != "foo-skill" {
		t.Errorf("foo/SKILL.md = %q", got["foo"]["SKILL.md"])
	}
	if string(got["foo"]["references/extra.md"]) != "foo-extra" {
		t.Errorf("foo/references/extra.md = %q", got["foo"]["references/extra.md"])
	}
	if string(got["bar"]["SKILL.md"]) != "bar-skill" {
		t.Errorf("bar/SKILL.md = %q", got["bar"]["SKILL.md"])
	}
}

func TestWalkTarball_NonStandardWrapperDir(t *testing.T) {
	tarball := buildTarball(t, map[string]string{
		"some-other-prefix/plugins/agent-manager/skills/foo/SKILL.md": "foo-skill",
	})

	got, err := walkTarball(tarball, "plugins/agent-manager/skills/")
	if err != nil {
		t.Fatalf("walkTarball: %v", err)
	}
	if string(got["foo"]["SKILL.md"]) != "foo-skill" {
		t.Errorf("foo/SKILL.md not picked up under non-standard wrapper: got %q", got["foo"]["SKILL.md"])
	}
}

func TestWalkTarball_NoMatchingEntries(t *testing.T) {
	tarball := buildTarball(t, map[string]string{
		"repo-main/README.md": "x",
	})
	got, err := walkTarball(tarball, "plugins/agent-manager/skills/")
	if err != nil {
		t.Fatalf("walkTarball: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty map, got %d entries", len(got))
	}
}

func TestWalkTarball_MalformedGzip(t *testing.T) {
	_, err := walkTarball([]byte("not a gzip"), "plugins/agent-manager/skills/")
	if err == nil {
		t.Fatal("want error for malformed gzip, got nil")
	}
}

func keysOf(m map[string]map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
