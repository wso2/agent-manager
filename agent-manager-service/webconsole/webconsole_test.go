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

package webconsole

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func files() fstest.MapFS {
	return fstest.MapFS{
		"index.html":          {Data: []byte("<html>app</html>")},
		"assets/index-abc.js": {Data: []byte("console.log(1)")},
		"favicon.ico":         {Data: []byte("ico")},
		"config.template.js":  {Data: []byte("x = { id: '$AUTH_CLIENT_ID', s: '$AUTH_SCOPES', u: '${API_BASE_URL}', d: '$DOCS_URL', q: '$UNSET' };")},
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "console.example:9000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandlerServesFilesAndFallsBackToIndex(t *testing.T) {
	h := handler(files())
	cases := []struct {
		path, body, cache string
		status            int
	}{
		{"/", "<html>app</html>", "no-cache", 200},
		{"/org/default/projects/x", "<html>app</html>", "no-cache", 200},
		{"/assets/index-abc.js", "console.log(1)", "public, max-age=31536000, immutable", 200},
		{"/favicon.ico", "ico", "public, max-age=3600", 200},
		{"/assets/missing.js", "", "", 404},
		{"/api/unknown", "", "", 404},
		{"/auth/whatever", "", "", 404},
		{"/mcp", "", "", 404},
	}
	for _, c := range cases {
		rec := get(t, h, c.path)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.path, rec.Code, c.status)
			continue
		}
		if c.status != 200 {
			continue
		}
		if got := rec.Body.String(); got != c.body {
			t.Errorf("%s: body %q, want %q", c.path, got, c.body)
		}
		if got := rec.Header().Get("Cache-Control"); got != c.cache {
			t.Errorf("%s: Cache-Control %q, want %q", c.path, got, c.cache)
		}
	}
}

func TestConfigJSRendersSettings(t *testing.T) {
	env := map[string]string{"CONSOLE_AUTH_CLIENT_ID": "my-client", "CONSOLE_DOCS_URL": "it's"}
	configValue = func(k string) string { return env[k] }
	defer func() { configValue = osGetenv }()

	rec := get(t, handler(files()), "/config.js")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"id: 'my-client'",
		"u: 'http://console.example:9000'",
		`d: 'it\'s'`,
		"q: ''",
		"s: 'openid profile email",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("config.js lacks %q:\n%s", want, body)
		}
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control %q", got)
	}
}

func TestPlaceholderWithoutConsole(t *testing.T) {
	rec := get(t, handler(nil), "/org/x")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "built without the console") {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if rec := get(t, handler(nil), "/api/v2/x"); rec.Code != 404 {
		t.Fatalf("api path: %d", rec.Code)
	}
}

func TestJSEscape(t *testing.T) {
	if got := jsEscape(`a'b\c</script>` + "\n"); got != `a\'b\\c\x3c/script>\n` {
		t.Fatalf("got %s", got)
	}
}
