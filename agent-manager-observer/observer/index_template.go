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

package observer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

// tracesTemplate maps span IDs and string attributes as keywords, which the
// observer's term filters and aggregations need. It must be in place before
// the collector creates the first span index.
//
//go:embed traces-template.json
var tracesTemplate []byte

// indexTemplateBody is the template with its index pattern set to the trace
// index the observer reads.
func indexTemplateBody(traceIndex string) ([]byte, error) {
	var tmpl map[string]any
	if err := json.Unmarshal(tracesTemplate, &tmpl); err != nil {
		return nil, fmt.Errorf("decode embedded index template: %w", err)
	}
	tmpl["index_patterns"] = []string{traceIndex}
	return json.Marshal(tmpl)
}

// EnsureIndexTemplate puts the trace index template, retrying until it
// succeeds or ctx ends, so it can run while OpenSearch is still starting.
// An empty template name turns it off.
func EnsureIndexTemplate(ctx context.Context, cfg config.OpenSearchConfig, retryEvery time.Duration) error {
	if cfg.IndexTemplate == "" || cfg.Address == "" {
		return nil
	}
	body, err := indexTemplateBody(cfg.TraceIndex)
	if err != nil {
		return err
	}
	c, _ := NewClient(cfg).(*clientImpl)
	url := c.baseURL + "/_index_template/" + cfg.IndexTemplate
	for {
		err = c.put(ctx, url, body)
		if err == nil {
			slog.Info("Trace index template applied", "template", cfg.IndexTemplate, "indexPattern", cfg.TraceIndex)
			return nil
		}
		slog.Warn("Applying the trace index template failed; retrying", "template", cfg.IndexTemplate, "error", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("apply index template %q: %w", cfg.IndexTemplate, err)
		case <-time.After(retryEvery):
		}
	}
}

func (c *clientImpl) put(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, msg)
	}
	return nil
}
