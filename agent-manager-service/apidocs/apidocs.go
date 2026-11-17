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

// Package apidocs serves the API reference at /api/docs: a Redoc page
// rendering GET /api/v1/openapi.json, the spec of the running server. The
// viewer is embedded, so the page works without network access, and its
// Content-Security-Policy keeps the browser from loading anything that is not
// served here.
package apidocs

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// RedocVersion is the Redoc release embedded in viewer/ (npm redoc,
// bundles/redoc.standalone.js, MIT). viewer/README.md says how to update it.
const RedocVersion = "2.5.4"

// bundleName is the viewer script's file name; the version in it lets the
// browser cache it for good.
const bundleName = "redoc-" + RedocVersion + ".standalone.js"

//go:embed viewer
var viewer embed.FS

// The page is relative to /api/docs: the script is /api/docs/<bundle> and the
// spec /api/v1/openapi.json.
var page = []byte(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Agent Manager API</title>
<style>body { margin: 0; }</style>
</head>
<body>
<redoc spec-url="v1/openapi.json" expand-responses="200,201" required-props-first="true"></redoc>
<script src="docs/` + bundleName + `"></script>
</body>
</html>
`)

// contentSecurityPolicy allows only what Redoc needs from this server: its
// script, its inline styles, its search worker (a blob URL) and the spec.
// Redoc's footer links a logo on a CDN; img-src keeps the browser from
// fetching it (Redoc hides the image when it fails to load).
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

// Page serves the reference page.
func Page() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-cache")
		_, _ = w.Write(page)
	})
}

// Files serves the viewer's files under /api/docs/{file}: the script, and
// the licence texts that ship with it.
func Files() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("file")
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		switch {
		case name == bundleName:
			h.Set("Content-Type", "text/javascript; charset=utf-8")
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
			h.Add("Vary", "Accept-Encoding")
			gz, err := viewer.ReadFile("viewer/" + bundleName + ".gz")
			if err != nil {
				http.Error(w, "viewer unavailable", http.StatusInternalServerError)
				return
			}
			if acceptsGzip(r) {
				h.Set("Content-Encoding", "gzip")
				_, _ = w.Write(gz)
				return
			}
			zr, err := gzip.NewReader(bytes.NewReader(gz))
			if err != nil {
				http.Error(w, "viewer unavailable", http.StatusInternalServerError)
				return
			}
			defer func() { _ = zr.Close() }()
			_, _ = io.Copy(w, zr) //nolint:gosec // decompressing our own embedded file, of known size
		case strings.HasPrefix(name, "LICENSE") || strings.HasSuffix(name, ".LICENSE.txt"):
			data, err := viewer.ReadFile("viewer/" + name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			h.Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	})
}

// acceptsGzip reports whether the request accepts gzip (or any encoding)
// with a non-zero weight.
func acceptsGzip(r *http.Request) bool {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		enc, params, _ := strings.Cut(part, ";")
		enc = strings.TrimSpace(enc)
		if !strings.EqualFold(enc, "gzip") && enc != "*" {
			continue
		}
		q, ok := strings.CutPrefix(strings.ReplaceAll(params, " ", ""), "q=")
		if !ok {
			return true
		}
		w, err := strconv.ParseFloat(q, 64)
		return err == nil && w > 0
	}
	return false
}
