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
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// defaultArchiveURL is the canonical location of the agent-skills repo
// tarball. Tracks main; for stable releases switch this to a tag URL.
const defaultArchiveURL = "https://github.com/wso2/agent-skills/archive/refs/heads/main.tar.gz"

// remotePathPrefix is the subtree within the archive that contains
// amctl's skill set.
const remotePathPrefix = "plugins/agent-manager/skills/"

// Remote downloads the agent-skills tarball using the supplied client
// and returns an in-memory fs.FS whose layout matches what the previously
// embedded FS exposed:
//
//	skilldata/
//	  <skill-name>/
//	    SKILL.md
//	    ...
func Remote(ctx context.Context, client *http.Client) (fs.FS, error) {
	return remoteFrom(ctx, client, defaultArchiveURL, remotePathPrefix)
}

// remoteFrom is the testable seam: same as Remote but takes an explicit
// URL and prefix so unit tests can point at an httptest server.
func remoteFrom(ctx context.Context, client *http.Client, url, pathPrefix string) (fs.FS, error) {
	tarball, err := fetchTarball(ctx, client, url)
	if err != nil {
		return nil, err
	}
	skills, err := walkTarball(tarball, pathPrefix)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("no skills found under %q in archive", pathPrefix)
	}

	files := make(map[string][]byte)
	for skillName, perSkill := range skills {
		for relative, contents := range perSkill {
			files["skilldata/"+skillName+"/"+relative] = contents
		}
	}
	return memFS(files), nil
}

// memFS is a minimal in-memory fs.FS used by Remote. Keys are full
// slash-separated paths (e.g. "skilldata/foo/SKILL.md"). Directories
// are inferred from key prefixes — no explicit directory entries.
type memFS map[string][]byte

func (m memFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &memDir{fs: m, prefix: ""}, nil
	}
	if data, ok := m[name]; ok {
		return &memFile{name: name, data: bytes.NewReader(data), size: int64(len(data))}, nil
	}
	prefix := name + "/"
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			return &memDir{fs: m, prefix: prefix}, nil
		}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	var prefix string
	if name != "." {
		prefix = name + "/"
	}
	seen := map[string]bool{}
	var out []fs.DirEntry
	for k, v := range m {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := k[len(prefix):]
		slash := strings.IndexByte(rest, '/')
		if slash < 0 {
			out = append(out, &memDirEntry{name: rest, isDir: false, size: int64(len(v))})
			continue
		}
		dirName := rest[:slash]
		if seen[dirName] {
			continue
		}
		seen[dirName] = true
		out = append(out, &memDirEntry{name: dirName, isDir: true})
	}
	return out, nil
}

type memFile struct {
	name string
	data *bytes.Reader
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: f.name, size: f.size}, nil
}
func (f *memFile) Read(b []byte) (int, error) { return f.data.Read(b) }
func (f *memFile) Close() error               { return nil }

type memDir struct {
	fs     memFS
	prefix string
}

func (d *memDir) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: strings.TrimSuffix(d.prefix, "/"), isDir: true}, nil
}
func (d *memDir) Read(_ []byte) (int, error) { return 0, fmt.Errorf("is a directory") }
func (d *memDir) Close() error               { return nil }

type memDirEntry struct {
	name  string
	isDir bool
	size  int64
}

func (e *memDirEntry) Name() string      { return e.name }
func (e *memDirEntry) IsDir() bool       { return e.isDir }
func (e *memDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e *memDirEntry) Info() (fs.FileInfo, error) {
	return &memFileInfo{name: e.name, size: e.size, isDir: e.isDir}, nil
}

type memFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (i *memFileInfo) Name() string { return i.name }
func (i *memFileInfo) Size() int64  { return i.size }
func (i *memFileInfo) Mode() fs.FileMode {
	if i.isDir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (i *memFileInfo) ModTime() time.Time { return time.Time{} }
func (i *memFileInfo) IsDir() bool        { return i.isDir }
func (i *memFileInfo) Sys() any           { return nil }

// userAgent identifies amctl in outbound requests to GitHub.
const userAgent = "amctl"

func fetchTarball(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: http %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

// walkTarball decompresses a gzipped tar and returns files whose path
// contains pathPrefix, grouped by the first path segment after the prefix
// (the skill name). The returned map is: skillName -> relativePath -> contents.
//
// Directory entries and non-regular files are skipped. The top-level
// directory inside the tarball (e.g. "agent-skills-main/") is not
// hardcoded — pathPrefix is located with strings.Index, so any wrapper
// directory works.
func walkTarball(tarball []byte, pathPrefix string) (map[string]map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	skills := make(map[string]map[string][]byte)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar next: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		idx := strings.Index(hdr.Name, pathPrefix)
		if idx < 0 {
			continue
		}
		afterPrefix := hdr.Name[idx+len(pathPrefix):]
		slash := strings.IndexByte(afterPrefix, '/')
		if slash <= 0 {
			continue
		}
		skillName := afterPrefix[:slash]
		relative := afterPrefix[slash+1:]
		if relative == "" {
			continue
		}

		contents, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", hdr.Name, err)
		}

		if _, ok := skills[skillName]; !ok {
			skills[skillName] = make(map[string][]byte)
		}
		skills[skillName][relative] = contents
	}

	return skills, nil
}
