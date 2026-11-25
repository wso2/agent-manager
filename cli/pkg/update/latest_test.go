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

package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// page is one canned GitHub response.
type page struct {
	status int
	body   string
	link   string
}

// fakeGitHub serves pages in order and records the URLs requested.
func fakeGitHub(pages ...page) (*http.Client, *[]string) {
	var urls []string
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		urls = append(urls, r.URL.String())
		if len(urls) > len(pages) {
			return nil, fmt.Errorf("unexpected request %d: %s", len(urls), r.URL)
		}
		p := pages[len(urls)-1]
		h := http.Header{}
		if p.link != "" {
			h.Set("Link", p.link)
		}
		return &http.Response{StatusCode: p.status, Body: io.NopCloser(strings.NewReader(p.body)), Header: h}, nil
	})}, &urls
}

func platformAsset(version string) string {
	return fmt.Sprintf(`{"name":"amctl_v%s_%s_%s.tar.gz","browser_download_url":"https://dl/%s/%s_%s"}`,
		version, runtime.GOOS, runtime.GOARCH, version, runtime.GOOS, runtime.GOARCH)
}

func release(tag, extra string) string {
	return fmt.Sprintf(`{"tag_name":%q,"html_url":"https://gh/%s",%s}`, tag, tag, extra)
}

func TestLatest_PicksHighestAndLinksThisPlatformsBinary(t *testing.T) {
	body := "[" + strings.Join([]string{
		release("amctl/v1.2.0", `"assets":[`+platformAsset("1.2.0")+`]`),
		release("amctl/v1.10.0", `"assets":[`+platformAsset("1.10.0")+`]`), // numeric, not lexical
		release("amctl/v1.3.0", `"assets":[`+platformAsset("1.3.0")+`]`),
	}, ",") + "]"
	client, urls := fakeGitHub(page{status: 200, body: body})

	got, err := Latest(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.10.0" || got.URL != fmt.Sprintf("https://dl/1.10.0/%s_%s", runtime.GOOS, runtime.GOARCH) {
		t.Errorf("got %+v", got)
	}
	if len(*urls) != 1 || !strings.HasPrefix((*urls)[0], "https://api.github.com/repos/wso2/agent-manager/releases") {
		t.Errorf("urls = %v", *urls)
	}
}

func TestLatest_SkipsWhatUsersCannotInstall(t *testing.T) {
	body := "[" + strings.Join([]string{
		release("amctl/v9.0.0", `"draft":true,"assets":[`+platformAsset("9.0.0")+`]`),
		release("amctl/v8.0.0-rc.1", `"prerelease":true,"assets":[`+platformAsset("8.0.0-rc.1")+`]`),
		release("amp/v7.0.0", `"assets":[`+platformAsset("7.0.0")+`]`),                                          // the platform's stream
		release("amctl/v6.0.0", `"assets":[]`),                                                                  // no binaries
		release("amctl/vNOPE", `"assets":[`+platformAsset("NOPE")+`]`),                                          // not a version
		release("amctl/v1.0.0", `"assets":[{"name":"amctl_v1.0.0_plan9_mips.zip","browser_download_url":"x"}]`), // other platform only
	}, ",") + "]"
	client, _ := fakeGitHub(page{status: 200, body: body})

	got, err := Latest(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	// 1.0.0 ships, but not for this platform: link the release page instead.
	if got.Version != "1.0.0" || got.URL != "https://gh/amctl/v1.0.0" {
		t.Errorf("got %+v", got)
	}
}

func TestLatest_FollowsPagesUntilARelease(t *testing.T) {
	client, urls := fakeGitHub(
		page{status: 200, body: "[" + release("amp/v5.0.0", `"assets":[]`) + "]",
			link: `<https://api.github.com/page2>; rel="next", <https://api.github.com/last>; rel="last"`},
		page{status: 200, body: "[" + release("amctl/v1.1.0", `"assets":[`+platformAsset("1.1.0")+`]`) + "]",
			link: `<https://api.github.com/page3>; rel="next"`},
		// A third page must never be requested: the first amctl release ends the walk.
	)
	got, err := Latest(context.Background(), client)
	if err != nil || got.Version != "1.1.0" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if len(*urls) != 2 || (*urls)[1] != "https://api.github.com/page2" {
		t.Errorf("urls = %v", *urls)
	}
}

func TestLatest_Failures(t *testing.T) {
	cases := []struct {
		name    string
		pages   []page
		wantErr string
	}{
		{"no amctl release anywhere", []page{{status: 200, body: "[]"}}, "no published amctl release"},
		{"rate limited", []page{{status: 403, body: "{}"}}, "status 403"},
		{"garbage body", []page{{status: 200, body: "<html>"}}, "decode releases"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := fakeGitHub(tc.pages...)
			if _, err := Latest(context.Background(), client); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}

	t.Run("network down", func(t *testing.T) {
		client, _ := fakeGitHub() // any request errors
		if _, err := Latest(context.Background(), client); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("page cap", func(t *testing.T) {
		pages := make([]page, 0, maxPages+2)
		for i := 0; i < maxPages+2; i++ {
			pages = append(pages, page{status: 200, body: "[]", link: `<https://api.github.com/next>; rel="next"`})
		}
		client, urls := fakeGitHub(pages...)
		if _, err := Latest(context.Background(), client); err == nil {
			t.Error("expected error")
		}
		if len(*urls) != maxPages {
			t.Errorf("walked %d pages, cap is %d", len(*urls), maxPages)
		}
	})
}

func TestNextLink(t *testing.T) {
	for in, want := range map[string]string{
		"":                          "",
		`<https://a/2>; rel="next"`: "https://a/2",
		`<https://a/1>; rel="prev", <https://a/3>; rel="next"`: "https://a/3",
		`<https://a/9>; rel="last"`:                            "",
		"garbage":                                              "",
	} {
		if got := nextLink(in); got != want {
			t.Errorf("nextLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{"1.2.3": true, "v1.2.3": true, "1.0.0-rc.1": true, "dev": false, "": false, "1.2": false, "1.x.3": false} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}
