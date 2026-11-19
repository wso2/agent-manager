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

// Package webconsole serves the console from the API port. The built console
// (console/apps/web-ui/dist) is embedded when the binary is built with the
// "console" build tag (see the Dockerfile and `make console-embed`); without
// it the binary serves a page saying so, so backend builds and unit tests need
// no Node toolchain.
//
// config.js, which the console reads before it starts, is rendered here from
// config.template.js: every $NAME in the template is CONSOLE_<NAME> from the
// configuration (amp.yaml `console:` or the environment), else a default
// derived from the server's own settings, else empty.
package webconsole

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/internal/reqorigin"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// Embedded reports whether this binary carries a built console.
func Embedded() bool { return dist() != nil }

// reserved paths belong to the API; the console never answers them, so a
// typo in an API path is a 404 rather than the console's index page.
var reserved = []string{"/api/", "/m/", "/auth/", "/mcp", "/.well-known/", "/healthz"}

// Handler serves the console: files from the build, config.js rendered from
// the configuration, and index.html for every other path (client-side routes).
func Handler() http.Handler { return handler(dist()) }

func handler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		for _, p := range reserved {
			if strings.HasPrefix(r.URL.Path, p) || r.URL.Path == strings.TrimSuffix(p, "/") {
				http.NotFound(w, r)
				return
			}
		}
		if files == nil {
			servePlaceholder(w)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		switch name {
		case "config.js":
			serveConfig(w, r, files)
			return
		case "", "index.html":
			serveIndex(w, r, files)
			return
		}
		if st, err := fs.Stat(files, name); err == nil && !st.IsDir() {
			serveFile(w, r, files, name)
			return
		}
		// A missing file (it has an extension) is a 404; anything else is a
		// client-side route, answered by the app.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, r, files)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, files fs.FS) {
	w.Header().Set("Cache-Control", "no-cache")
	serveContent(w, r, files, "index.html")
}

func serveFile(w http.ResponseWriter, r *http.Request, files fs.FS, name string) {
	if strings.HasPrefix(name, "assets/") {
		// Vite puts a content hash in every file name under assets/.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	serveContent(w, r, files, name)
}

func serveContent(w http.ResponseWriter, r *http.Request, files fs.FS, name string) {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

var placeholder = regexp.MustCompile(`\$(?:\{(\w+)\}|(\w+))`)

func serveConfig(w http.ResponseWriter, r *http.Request, files fs.FS) {
	tmpl, err := fs.ReadFile(files, "config.template.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defaults := defaultValues(r)
	out := placeholder.ReplaceAllFunc(tmpl, func(m []byte) []byte {
		sub := placeholder.FindSubmatch(m)
		name := string(sub[1])
		if name == "" {
			name = string(sub[2])
		}
		return []byte(jsEscape(Value(name, defaults)))
	})
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}

// Value is the console setting name: CONSOLE_<name> from the configuration,
// else defaults[name], else empty.
func Value(name string, defaults map[string]string) string {
	if v := strings.TrimSpace(configValue("CONSOLE_" + name)); v != "" {
		return v
	}
	return defaults[name]
}

// configValue reads a setting; amp.yaml has already been folded into the
// environment by package config.
var configValue = config.Value

// defaultValues are the console settings the server can work out itself.
func defaultValues(r *http.Request) map[string]string {
	cfg := config.GetConfig()
	origin := strings.TrimRight(cfg.ServerPublicURL, "/")
	if origin == "" {
		origin = reqorigin.Of(r)
	}
	return map[string]string{
		"API_BASE_URL":          origin,
		"AUTH_BASE_URL":         cfg.Thunder.BaseURL,
		"AUTH_CLIENT_ID":        "amp-console-client",
		"AUTH_MODE":             cfg.AuthMode,
		"AUTH_SCOPES":           Scopes(),
		"SIGN_IN_REDIRECT_URL":  origin + "/login",
		"SIGN_OUT_REDIRECT_URL": origin + "/login",
		"BASE_PATH":             "/",
		"AMP_VERSION":           cfg.PackageVersion,
		"DOCS_URL":              "https://wso2.com/agent-platform/docs/next",
	}
}

// Scopes is the scope list the console asks for when it signs in: the OIDC
// basics and the scope of every permission the core and its active modules
// declare, so a module's pages work without a console setting naming them.
func Scopes() string {
	perms := rbac.Current().Permissions()
	scopes := make([]string, 0, len(perms)+3)
	scopes = append(scopes, "openid", "profile", "email")
	for _, p := range perms {
		scopes = append(scopes, p.Scope())
	}
	return strings.Join(scopes, " ")
}

// jsEscape makes s safe inside a single-quoted JavaScript string in a script
// file.
func jsEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '<':
			b.WriteString(`\x3c`)
		case ' ':
			b.WriteString(` `)
		case ' ':
			b.WriteString(` `)
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

const placeholderPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Agent Manager</title></head>
<body style="font-family: sans-serif; max-width: 40em; margin: 4em auto">
<h1>Agent Manager</h1>
<p>The API is running, but this binary was built without the console.</p>
<p>Build it with the console (<code>make console-embed build</code> in agent-manager-service, or the
Docker image), or run the console from source with <code>make console</code>.</p>
</body></html>
`

func servePlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(placeholderPage))
}
