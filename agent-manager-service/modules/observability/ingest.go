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

package observability

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

const (
	// IngestPath is the OTLP/HTTP traces route; an agent's OTLP endpoint is
	// <amp>/m/observability/otel.
	IngestPath = "/otel/v1/traces"
	// DefaultMaxBodyBytes caps an export's body as sent, compressed or not.
	DefaultMaxBodyBytes = 10 << 20
	// agentCacheTTL is how long an agent's existence is remembered.
	agentCacheTTL = time.Minute
	// maxResponseBytes caps the collector's response passed back.
	maxResponseBytes = 1 << 20
)

// identityHeaders are the headers the collector takes a span's identity
// from, set from the agent key. Written lowercase, as the collector names them.
var identityHeaders = []string{
	"x-org-id", "x-user-project", "x-user-component", "x-user-environment",
	"x-user-organization-name", "x-user-project-name", "x-user-agent-name",
}

func identity(k modkit.AgentKey) map[string]string {
	v := []string{k.OrgID, k.ProjectID, k.AgentID, k.EnvironmentID, k.OrgName, k.ProjectName, k.AgentName}
	out := make(map[string]string, len(identityHeaders))
	for i, h := range identityHeaders {
		out[h] = v[i]
	}
	return out
}

// agentLookup finds an agent in the core.
type agentLookup interface {
	GetAgent(ctx context.Context, orgID, agentID string) (modkit.Agent, error)
}

// ingest forwards agents' OTLP/HTTP trace exports to the collector, with the
// identity headers set from the verified agent key and nothing else the agent
// sent but the body and its type and encoding.
type ingest struct {
	// traces is the collector's traces URL; empty when none is configured.
	traces  string
	maxBody int64
	client  *http.Client
	agents  *agentCache
	log     *slog.Logger
	// warned is when the last "no collector" line was logged (unix seconds).
	warned atomic.Int64
}

// newIngest reads collector.url and ingest.max-body-bytes.
func newIngest(rt modkit.Runtime) (*ingest, error) {
	in := &ingest{
		maxBody: DefaultMaxBodyBytes,
		client: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		agents: &agentCache{core: rt.Core(), seen: map[string]agentSeen{}},
		log:    rt.Logger(),
	}
	if s := rt.Config("ingest.max-body-bytes"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("observability: ingest.max-body-bytes %q must be a positive number of bytes", s)
		}
		in.maxBody = n
	}
	base := rt.Config("collector.url")
	if base == "" {
		rt.Logger().Warn("observability: no collector configured (MODULE_OBSERVABILITY_COLLECTOR_URL); trace exports are refused with 503")
		return in, nil
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("observability: collector.url %q must be an http or https URL", base)
	}
	in.traces = strings.TrimRight(base, "/") + "/v1/traces"
	rt.Logger().Info("observability: forwarding trace exports to the collector", "url", in.traces)
	return in, nil
}

func (in *ingest) serve(w http.ResponseWriter, r *http.Request) {
	key, ok := modkit.AgentKeyFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "an agent key is required")
		return
	}
	modkit.AuditResource(r.Context(), "agent", key.AgentID, key.AgentName)
	if in.traces == "" {
		if now := time.Now().Unix(); in.warned.Load() <= now-60 {
			in.warned.Store(now)
			in.log.Warn("observability: refused a trace export: no collector configured (MODULE_OBSERVABILITY_COLLECTOR_URL)")
		}
		writeError(w, http.StatusServiceUnavailable, "no trace collector is configured")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/x-protobuf" && mediaType != "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "send OTLP/HTTP traces as application/x-protobuf or application/json")
		return
	}
	encoding := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))
	if encoding != "" && encoding != "identity" && encoding != "gzip" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Encoding must be gzip or none")
		return
	}
	if r.ContentLength > in.maxBody {
		writeTooLarge(w, in.maxBody)
		return
	}
	exists, err := in.agents.exists(r.Context(), key.OrgID, key.AgentID)
	if err != nil {
		in.log.Error("observability: could not look up the exporting agent", "agentId", key.AgentID, "error", err)
		writeError(w, http.StatusServiceUnavailable, "the agent could not be checked; retry")
		return
	}
	if !exists {
		writeError(w, http.StatusUnauthorized, "the agent this key belongs to no longer exists")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, in.maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeTooLarge(w, in.maxBody)
			return
		}
		writeError(w, http.StatusBadRequest, "the request body could not be read")
		return
	}
	in.forward(w, r.Context(), key, r.Header.Get("Content-Type"), encoding, body)
}

// forward posts body to the collector and passes its answer back.
func (in *ingest) forward(w http.ResponseWriter, ctx context.Context, key modkit.AgentKey, contentType, encoding string, body []byte) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, in.traces, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "the export could not be forwarded")
		return
	}
	// A fresh header set: nothing the agent sent reaches the collector but
	// the body's type and encoding.
	req.Header = http.Header{"Content-Type": {contentType}}
	if encoding == "gzip" {
		req.Header.Set("Content-Encoding", "gzip")
	}
	for h, v := range identity(key) {
		req.Header[h] = []string{v}
	}
	resp, err := in.client.Do(req)
	if err != nil {
		in.log.Warn("observability: the collector is unreachable", "url", in.traces, "error", err)
		writeError(w, http.StatusServiceUnavailable, "the trace collector is unreachable; retry")
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxResponseBytes))
}

func writeTooLarge(w http.ResponseWriter, limit int64) {
	writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("an export may be at most %d bytes; send smaller batches", limit))
}

// agentCache remembers for a minute whether an agent exists, so a deleted
// agent's key stops working without a core lookup per export.
type agentCache struct {
	core agentLookup
	mu   sync.Mutex
	seen map[string]agentSeen
}

type agentSeen struct {
	exists bool
	until  time.Time
}

func (c *agentCache) exists(ctx context.Context, orgID, agentID string) (bool, error) {
	now, id := time.Now(), orgID+"/"+agentID
	c.mu.Lock()
	s, ok := c.seen[id]
	c.mu.Unlock()
	if ok && now.Before(s.until) {
		return s.exists, nil
	}
	_, err := c.core.GetAgent(ctx, orgID, agentID)
	if err != nil && !errors.Is(err, modkit.ErrNotFound) {
		return false, err
	}
	exists := err == nil
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.seen) > 10000 {
		for k, e := range c.seen {
			if now.After(e.until) {
				delete(c.seen, k)
			}
		}
	}
	c.seen[id] = agentSeen{exists: exists, until: now.Add(agentCacheTTL)}
	return exists, nil
}
