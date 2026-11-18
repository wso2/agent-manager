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

package apidocs

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bundleSHA256 is the SHA-256 of the uncompressed Redoc script, as published
// in the npm package (see viewer/README.md).
const bundleSHA256 = "dcaf76612bc4a3fbcc923a8966dee2f6146a5f32e5ce1b6f02dd60cbbf89500b"

func serve(t *testing.T, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /api/docs", Page())
	mux.Handle("GET /api/docs/{file}", Files())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestPagePointsAtTheBundleAndTheSpec(t *testing.T) {
	rec := serve(t, "/api/docs", "")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `spec-url="v1/openapi.json"`)
	assert.Contains(t, body, `src="docs/`+bundleName+`"`)
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "default-src 'none'")
	assert.NotContains(t, body, "http", "the page must not load anything from elsewhere")
}

func TestBundleIsTheRecordedRelease(t *testing.T) {
	plain := serve(t, "/api/docs/"+bundleName, "")
	require.Equal(t, http.StatusOK, plain.Code)
	assert.Empty(t, plain.Header().Get("Content-Encoding"))
	sum := sha256.Sum256(plain.Body.Bytes())
	assert.Equal(t, bundleSHA256, hex.EncodeToString(sum[:]))
	assert.True(t, strings.HasPrefix(plain.Header().Get("Content-Type"), "text/javascript"))

	gz := serve(t, "/api/docs/"+bundleName, "br, gzip;q=0.8")
	require.Equal(t, http.StatusOK, gz.Code)
	assert.Equal(t, "gzip", gz.Header().Get("Content-Encoding"))
	zr, err := gzip.NewReader(bytes.NewReader(gz.Body.Bytes()))
	require.NoError(t, err)
	unzipped, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, plain.Body.Bytes(), unzipped)

	refused := serve(t, "/api/docs/"+bundleName, "gzip;q=0")
	assert.Empty(t, refused.Header().Get("Content-Encoding"))
}

func TestFilesServesOnlyTheViewer(t *testing.T) {
	assert.Equal(t, http.StatusOK, serve(t, "/api/docs/LICENSE.redoc", "").Code)
	assert.Equal(t, http.StatusOK, serve(t, "/api/docs/"+bundleName+".LICENSE.txt", "").Code)
	for _, p := range []string{"/api/docs/README.md", "/api/docs/" + bundleName + ".gz", "/api/docs/redoc.standalone.js"} {
		assert.Equal(t, http.StatusNotFound, serve(t, p, "").Code, p)
	}
}
