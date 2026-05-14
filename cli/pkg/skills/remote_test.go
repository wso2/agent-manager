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
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
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

func TestFetchTarball_ReturnsBodyOn200(t *testing.T) {
	want := []byte("fake tarball bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("missing User-Agent header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	got, err := fetchTarball(context.Background(), http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("fetchTarball: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("body mismatch: got %q want %q", got, want)
	}
}

func TestFetchTarball_ErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchTarball(context.Background(), http.DefaultClient, srv.URL)
	if err == nil {
		t.Fatal("want error for 404, got nil")
	}
}

func TestFetchTarball_RespectsCanceledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchTarball(ctx, http.DefaultClient, srv.URL)
	if err == nil {
		t.Fatal("want error from canceled context, got nil")
	}
}

func TestRemote_ReturnsFSWithSkilldataLayout(t *testing.T) {
	tarball := buildTarball(t, map[string]string{
		"agent-skills-main/plugins/agent-manager/skills/foo/SKILL.md":           "foo-content",
		"agent-skills-main/plugins/agent-manager/skills/foo/references/note.md": "note-content",
		"agent-skills-main/plugins/agent-manager/skills/bar/SKILL.md":           "bar-content",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	fsys, err := remoteFrom(context.Background(), http.DefaultClient, srv.URL, "plugins/agent-manager/skills/")
	if err != nil {
		t.Fatalf("remoteFrom: %v", err)
	}

	entries, err := fs.ReadDir(fsys, "skilldata")
	if err != nil {
		t.Fatalf("ReadDir skilldata: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 skills under skilldata/, got %d", len(entries))
	}

	got, err := fs.ReadFile(fsys, "skilldata/foo/SKILL.md")
	if err != nil {
		t.Fatalf("ReadFile foo/SKILL.md: %v", err)
	}
	if string(got) != "foo-content" {
		t.Errorf("foo/SKILL.md = %q, want %q", got, "foo-content")
	}

	got, err = fs.ReadFile(fsys, "skilldata/foo/references/note.md")
	if err != nil {
		t.Fatalf("ReadFile foo/references/note.md: %v", err)
	}
	if string(got) != "note-content" {
		t.Errorf("foo/references/note.md = %q, want %q", got, "note-content")
	}
}

func TestRemote_FailsWhenNoSkillsFound(t *testing.T) {
	tarball := buildTarball(t, map[string]string{
		"agent-skills-main/README.md": "x",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	_, err := remoteFrom(context.Background(), http.DefaultClient, srv.URL, "plugins/agent-manager/skills/")
	if err == nil {
		t.Fatal("want error when tarball contains no skills, got nil")
	}
}

func keysOf(m map[string]map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
