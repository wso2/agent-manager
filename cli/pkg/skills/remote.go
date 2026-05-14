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
	"net/http"
	"strings"
)

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
