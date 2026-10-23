// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package update finds the latest published amctl release.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
)

const (
	repo   = "wso2/agent-manager"
	stream = "amctl"

	releasesURL = "https://api.github.com/repos/" + repo + "/releases?per_page=100"
	tagPrefix   = stream + "/v"
	assetPrefix = stream + "_v"
	// maxPages caps the paged release walk.
	maxPages = 10
)

// ReleasesPageURL lists amctl releases for users; /releases/latest is the platform release.
const ReleasesPageURL = "https://github.com/" + repo + "/releases?q=" + stream

// Release is a published amctl: its version and a download URL (the binary for this platform, or the release page).
type Release struct {
	Version string
	URL     string
}

// Latest returns the highest published, non-pre-release amctl release.
func Latest(ctx context.Context, httpc *http.Client) (Release, error) {
	// Releases are listed newest first, so stop after the first page with an amctl
	// release; later pages are never fetched once one is found.
	var latest Release
	next := releasesURL
	for page := 0; next != "" && page < maxPages && latest.Version == ""; page++ {
		batch, link, err := fetchReleases(ctx, httpc, next)
		if err != nil {
			return Release{}, err
		}
		latest = highest(latest, batch)
		next = link
	}
	if latest.Version == "" {
		return Release{}, fmt.Errorf("no published amctl release")
	}
	return latest, nil
}

// highest returns the highest published amctl release in rs, or best if none beats it.
func highest(best Release, rs []githubRelease) Release {
	platform := "_" + runtime.GOOS + "_" + runtime.GOARCH + "."
	for _, r := range rs {
		if r.Draft || r.Prerelease {
			continue
		}
		version, ok := streamVersion(r.TagName)
		if !ok {
			continue
		}
		if best.Version != "" && !Newer(best.Version, version) {
			continue
		}
		candidate := Release{Version: version}
		ships := false
		for _, a := range r.Assets {
			if !strings.HasPrefix(a.Name, assetPrefix+version+"_") {
				continue
			}
			ships = true
			if strings.Contains(a.Name, platform) {
				candidate.URL = a.DownloadURL
			}
		}
		if !ships {
			continue
		}
		if candidate.URL == "" {
			candidate.URL = r.HTMLURL
		}
		best = candidate
	}
	return best
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// fetchReleases returns one page of releases and the next page URL ("" on the last).
func fetchReleases(ctx context.Context, httpc *http.Client, url string) ([]githubRelease, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	var page []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("decode releases: %w", err)
	}
	return page, nextLink(resp.Header.Get("Link")), nil
}

// nextLink returns the rel="next" URL from a GitHub Link header, or "".
func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		target, params, ok := strings.Cut(part, ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(target), "<>")
	}
	return ""
}

func streamVersion(tag string) (string, bool) {
	version, ok := strings.CutPrefix(tag, tagPrefix)
	if !ok {
		return "", false
	}
	if _, valid := parse(version); !valid {
		return "", false
	}
	return version, true
}

// IsRelease reports whether v is a release version rather than e.g. a "dev" build.
func IsRelease(v string) bool {
	_, ok := parse(v)
	return ok
}

// Newer reports whether latest is newer than current. It returns false if either does not
// parse as a version, which is how "dev" builds skip the update warning.
func Newer(current, latest string) bool {
	c, ok := parse(current)
	if !ok {
		return false
	}
	l, ok := parse(latest)
	if !ok {
		return false
	}
	for i := range c.core {
		if c.core[i] != l.core[i] {
			return c.core[i] < l.core[i]
		}
	}
	// Same core: a pre-release (1.0.0-rc3) is older than its release (1.0.0).
	return c.pre != "" && l.pre == ""
}

type semver struct {
	core [3]int
	pre  string
}

func parse(v string) (semver, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var s semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		s.core[i] = n
	}
	s.pre = pre
	return s, true
}
