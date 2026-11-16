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

package google

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Fixtures (testdata/). They follow the schemas and constructed examples
// recorded in evidence/google.md:
//
//   - The service account key file is built in code (testKeyFile), in the
//     official key file format (docs.cloud.google.com/iam/docs/keys-create-delete,
//     evidence §4), with an RSA key generated at test time. It is not a
//     fixture file, so no credential-shaped file is checked in.
//   - token_response.json / token_error.json: OAuth token endpoint success
//     and "JWT error codes" shapes (developers.google.com service-account
//     page, evidence §4 and §2.7).
//   - engines_*.json: GoogleCloudAiplatformV1ListReasoningEnginesResponse,
//     the example constructed from the Discovery v1 schema (evidence §2.1).
//   - agents_*.json: v1 ListAgentsResponse, the constructed example with the
//     documented agentId URN, principal and attribute formats (§2.2).
//   - error_*.json: the AIP-193 error envelope (§2.7).
//
// Marker strings (SECRET-PROMPT-MARKER, sk-test-SECRETSECRETSECRET) sit in
// env values, descriptions, Agent Card content, skill descriptions and
// URLs; no result or log line may contain them.

const (
	testProject       = "my-agents-project"
	testProjectNumber = "123456789012"
	fakeAccessToken   = "ya29.FAKE-ACCESS-TOKEN-0123456789"
	clientEmail       = "amp-shadow-discovery@my-agents-project.iam.gserviceaccount.com"

	engineA = "projects/123456789012/locations/us-central1/reasoningEngines/1234567890123456789"
	engineB = "projects/my-agents-project/locations/us-central1/reasoningEngines/9876543210987654321"
	engineC = "projects/123456789012/locations/us/reasoningEngines/5555555555555555555"
)

var retentionMarkers = []string{
	"SECRET-PROMPT-MARKER", "sk-test-SECRETSECRETSECRET", fakeAccessToken, "PRIVATE KEY", "systemPrompt",
}

var (
	keyOnce sync.Once
	testKey *rsa.PrivateKey
	keyPEM  string
)

// generatedKey returns an RSA key generated for the test run (never a real
// key) and its PKCS#8 PEM, the format Google issues.
func generatedKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			panic(err)
		}
		testKey = k
		keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	})
	return testKey, keyPEM
}

type recordedRequest struct {
	Route  string
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// fakeGoogle is one TLS server playing the OAuth token endpoint and the two
// Google APIs the connector calls (Agent Runtime and Agent Registry agents).
// Any other route, such as Cloud Monitoring, Cloud Logging or the registry's
// MCP servers, fails the test.
type fakeGoogle struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu       sync.Mutex
	reqs     []recordedRequest
	override map[string]http.HandlerFunc
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	key, _ := generatedKey(t)
	f := &fakeGoogle{t: t, key: key, override: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func routeOf(r *http.Request) string {
	p := r.URL.Path
	switch {
	case p == "/token":
		return "token"
	case strings.HasSuffix(p, "/reasoningEngines"):
		return "engines"
	case strings.HasSuffix(p, "/agents"):
		return "agents"
	default:
		return ""
	}
}

func (f *fakeGoogle) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	route := routeOf(r)
	f.mu.Lock()
	f.reqs = append(f.reqs, recordedRequest{Route: route, Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body})
	h := f.override[route]
	f.mu.Unlock()
	if route == "" {
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	if route != "token" && r.Header.Get("Authorization") != "Bearer "+fakeAccessToken {
		f.t.Errorf("%s: missing or wrong Authorization header", route)
	}
	if h != nil {
		h(w, r)
		return
	}
	f.defaultHandler(route)(w, r)
}

func (f *fakeGoogle) setOverride(route string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.override[route] = h
}

// requests returns the recorded requests of a route ("" = all API routes).
func (f *fakeGoogle) requests(route string) []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedRequest
	for _, r := range f.reqs {
		if (route == "" && r.Route != "token") || r.Route == route {
			out = append(out, r)
		}
	}
	return out
}

var locationInPath = regexp.MustCompile(`/locations/([^/]+)/`)

func (f *fakeGoogle) defaultHandler(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loc := ""
		if m := locationInPath.FindStringSubmatch(r.URL.Path); m != nil {
			loc = m[1]
		}
		token := r.URL.Query().Get("pageToken")
		switch route {
		case "token":
			f.serveToken(w, r)
		case "engines":
			switch {
			case loc == "us-central1" && token == "":
				serveFixture(w, "engines_us-central1_page1.json")
			case loc == "us-central1" && token == "engines-page-2":
				serveFixture(w, "engines_us-central1_page2.json")
			case loc == "us":
				serveFixture(w, "engines_us.json")
			default:
				serveJSON(w, `{}`)
			}
		case "agents":
			switch {
			case loc == "us-central1" && token == "":
				serveFixture(w, "agents_us-central1_page1.json")
			case loc == "us-central1" && token == "agents-page-2":
				serveFixture(w, "agents_us-central1_page2.json")
			case loc == "global":
				serveFixture(w, "agents_global.json")
			default:
				serveJSON(w, `{}`)
			}
		}
	}
}

// serveToken checks the JWT-bearer grant the way Google does: the form
// fields, the RS256 signature with the service account's public key, and
// the iss, scope and aud claims.
func (f *fakeGoogle) serveToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		f.t.Errorf("token: want a form POST, got %s %s", r.Method, r.Header.Get("Content-Type"))
	}
	if err := r.ParseForm(); err != nil {
		f.t.Errorf("token: %v", err)
	}
	if g := r.PostForm.Get("grant_type"); g != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		f.t.Errorf("token: grant_type = %q", g)
	}
	assertion := r.PostForm.Get("assertion")
	var claims struct {
		jwt.RegisteredClaims
		Scope string `json:"scope"`
	}
	tok, err := jwt.ParseWithClaims(assertion, &claims, func(*jwt.Token) (any, error) { return &f.key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuedAt())
	if err != nil || !tok.Valid {
		w.WriteHeader(http.StatusBadRequest)
		serveFixtureBody(w, "token_error.json")
		return
	}
	if kid, _ := tok.Header["kid"].(string); kid != "0123456789abcdef0123456789abcdef01234567" {
		f.t.Errorf("token: kid = %q", kid)
	}
	aud := []string(claims.Audience)
	if claims.Issuer != clientEmail || claims.Scope != sa.GoogleScope || len(aud) != 1 || aud[0] != f.srv.URL+"/token" {
		f.t.Errorf("token: claims iss=%q scope=%q aud=%v", claims.Issuer, claims.Scope, aud)
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > time.Hour {
		f.t.Errorf("token: exp must be at most 1 hour after iat")
	}
	serveFixture(w, "token_response.json")
}

func readFixture(name string) []byte {
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		panic(err)
	}
	return b
}

func serveFixture(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(readFixture(name))
}

func serveFixtureBody(w http.ResponseWriter, name string) {
	_, _ = w.Write(readFixture(name))
}

func serveJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func serveError(status int, fixture string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(readFixture(fixture))
	}
}

// testKeyFile is a service account key file in the official format, for a
// test-only account; PrivateKey is set by the caller to a generated key.
type testKeyFile struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
	AuthURI      string `json:"auth_uri"`
	TokenURI     string `json:"token_uri"`
}

// keyFileJSON returns the test key file with pemKey as its private key.
func keyFileJSON(t *testing.T, pemKey string) string {
	t.Helper()
	b, err := json.Marshal(testKeyFile{
		Type:         "service_account",
		ProjectID:    "my-agents-project",
		PrivateKeyID: "0123456789abcdef0123456789abcdef01234567",
		PrivateKey:   pemKey,
		ClientEmail:  "amp-shadow-discovery@my-agents-project.iam.gserviceaccount.com",
		ClientID:     "100000000000000000001",
		AuthURI:      "https://accounts.google.com/o/oauth2/auth",
		TokenURI:     "https://accounts.google.com/o/oauth2/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// serviceAccountJSON is the test key file with the generated key.
func serviceAccountJSON(t *testing.T) string {
	t.Helper()
	_, pemKey := generatedKey(t)
	return keyFileJSON(t, pemKey)
}

func (f *fakeGoogle) conn(t *testing.T) sa.Connection {
	t.Helper()
	return sa.Connection{
		Name:     "google-test",
		Provider: models.ShadowProviderGoogle,
		Config: models.ShadowConnectionConfig{
			Endpoints: map[string]string{
				sa.EPGoogleToken:         f.srv.URL + "/token",
				sa.EPGoogleAIPlatform:    f.srv.URL + "/regional/{location}",
				sa.EPGoogleAIPlatformRep: f.srv.URL + "/multiregion/{location}",
				sa.EPGoogleRegistry:      f.srv.URL,
			},
			ProjectIDs:                  []string{testProject},
			Locations:                   []string{"us-central1"},
			IncludeGlobalAndMultiRegion: true,
		},
		Secret: models.ShadowConnectionSecret{ServiceAccountJSON: serviceAccountJSON(t)},
	}
}

// testConnector returns a connector on the fake server's TLS client whose
// sleeps are recorded instead of waited, and the buffer its debug logs go
// to.
type testConnector struct {
	*Connector
	logs  *bytes.Buffer
	mu    sync.Mutex
	waits []time.Duration
}

func newTestConnector(f *fakeGoogle) *testConnector {
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := New(sa.NewHTTPClient(f.srv.Client().Transport), logger).(*Connector)
	tc := &testConnector{Connector: c, logs: logs}
	c.sleep = func(ctx context.Context, d time.Duration) error {
		tc.mu.Lock()
		tc.waits = append(tc.waits, d)
		tc.mu.Unlock()
		return ctx.Err()
	}
	return tc
}

func callResult(t *testing.T, res *sa.Result, call string) models.ShadowScanCallResult {
	t.Helper()
	for _, c := range res.Calls {
		if c.Call == call {
			return c
		}
	}
	t.Fatalf("call %q not recorded; calls: %+v", call, res.Calls)
	return models.ShadowScanCallResult{}
}

func findingOf(t *testing.T, res *sa.Result, kind, externalID string) *sa.Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.Kind == kind && f.ExternalID == externalID {
			return f
		}
	}
	t.Fatalf("no %s finding %q", kind, externalID)
	return nil
}

func signalOf(f *sa.Finding, code string) (sa.Signal, bool) {
	for _, s := range f.Signals {
		if s.Code == code {
			return s, true
		}
	}
	return sa.Signal{}, false
}
