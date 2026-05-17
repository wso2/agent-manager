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
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing/fstest"

	"github.com/wso2/agent-manager/cli/pkg/version"
)

// defaultArchiveURL is the canonical location of the agent-skills repo
// tarball. Tracks main; for stable releases switch this to a tag URL.
const defaultArchiveURL = "https://github.com/wso2/agent-skills/archive/refs/heads/main.tar.gz"

// remotePathPrefix is the subtree within the archive that contains
// amctl's skill set.
const remotePathPrefix = "plugins/agent-manager/skills/"

// Remote downloads the agent-skills tarball using the supplied client
// and returns an in-memory fs.FS rooted at "skilldata/<skill-name>/...".
func Remote(ctx context.Context, client *http.Client) (fs.FS, error) {
	return remoteFrom(ctx, client, defaultArchiveURL, remotePathPrefix)
}

// remoteFrom is the testable seam: same as Remote but takes an explicit
// URL and prefix so unit tests can point at an httptest server.
func remoteFrom(ctx context.Context, client *http.Client, url, pathPrefix string) (fs.FS, error) {
	body, err := fetchTarball(ctx, client, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	skills, err := walkTarball(body, pathPrefix)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("no skills found under %q in archive", pathPrefix)
	}

	files := fstest.MapFS{}
	for skillName, perSkill := range skills {
		for relative, contents := range perSkill {
			files["skilldata/"+skillName+"/"+relative] = &fstest.MapFile{Data: contents}
		}
	}
	return files, nil
}

func userAgent() string {
	return "amctl/" + version.Version
}

func fetchTarball(ctx context.Context, client *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("fetch %s: http %d: %s", url, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return resp.Body, nil
}

// walkTarball decompresses a gzipped tar stream and returns files whose
// path contains pathPrefix, grouped by the first path segment after the
// prefix (the skill name). The wrapper directory inside the tarball
// (e.g. "agent-skills-main/") is located by substring search, so any
// wrapper works.
func walkTarball(r io.Reader, pathPrefix string) (map[string]map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
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
		if !validSkillName(skillName) {
			return nil, fmt.Errorf("invalid skill name %q in archive entry %q", skillName, hdr.Name)
		}
		if !validSkillRelPath(relative) {
			return nil, fmt.Errorf("invalid relative path %q in archive entry %q", relative, hdr.Name)
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
