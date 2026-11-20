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

package opensearchreader

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

// Options says where OpenSearch is and how to turn names into ids.
type Options struct {
	Addr             string // https://localhost:9200
	Username         string
	Password         string
	IndexPattern     string // otel-traces-*
	InsecureTLS      bool   // local development only: the cluster certificate is self-signed
	DefaultNamespace string
	Resolver         scope.Resolver
	Timeout          time.Duration
}

var indexPatternRE = regexp.MustCompile(`^[A-Za-z0-9_.*,-]+$`)

type searcher struct {
	http  *http.Client
	base  string
	index string
	user  string
	pass  string
}

func newSearcher(o Options) (*searcher, error) {
	u, err := url.Parse(o.Addr)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid OpenSearch address %q (want http(s)://host:port)", o.Addr)
	}
	if !indexPatternRE.MatchString(o.IndexPattern) {
		return nil, fmt.Errorf("invalid OpenSearch index pattern %q", o.IndexPattern)
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tr := &http.Transport{
		// A page makes up to 10 calls at once. Keep the connections open, or every call pays a new TLS handshake.
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		// #nosec G402 -- opt-in for local development only (config refuses it otherwise)
		TLSClientConfig: &tls.Config{InsecureSkipVerify: o.InsecureTLS},
	}
	return &searcher{
		http: &http.Client{Transport: tr, Timeout: timeout},
		base: strings.TrimRight(o.Addr, "/"), index: o.IndexPattern, user: o.Username, pass: o.Password,
	}, nil
}

// search sends one _search request and decodes the JSON answer into out.
func (s *searcher) search(ctx context.Context, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/"+s.index+"/_search", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("opensearch request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("read opensearch answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opensearch answered %d: %.300s", resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}
