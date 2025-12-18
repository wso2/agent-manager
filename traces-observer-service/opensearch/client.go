// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package opensearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/config"
)

// Client wraps the OpenSearch client
type Client struct {
	client *opensearch.Client
	config *config.OpenSearchConfig
}

// NewClient creates a new OpenSearch client
func NewClient(cfg *config.OpenSearchConfig) (*Client, error) {
	// Create HTTP transport with TLS verification disabled
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	opensearchConfig := opensearch.Config{
		Addresses: []string{cfg.Address},
		Transport: transport,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}

	client, err := opensearch.NewClient(opensearchConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	// Test connection
	info, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to OpenSearch: %w", err)
	} else {
		log.Printf("Connected to OpenSearch, status: %s", info.Status())
	}

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// Search executes a search query against one or more indices
func (c *Client) Search(ctx context.Context, indices []string, query map[string]interface{}) (*SearchResponse, error) {
	log.Printf("Executing search on indices: %v", indices)

	// Convert query to JSON
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	// Create search request with IgnoreUnavailable option
	req := opensearchapi.SearchRequest{
		Index:             indices,
		Body:              &buf,
		IgnoreUnavailable: opensearchapi.BoolPtr(true),
	}

	// Execute search
	res, err := req.Do(ctx, c.client)
	if err != nil {
		log.Printf("Search request failed: %v", err)
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Printf("Search request returned error: %s", res.Status())
		return nil, fmt.Errorf("search request failed with status: %s", res.Status())
	}

	// Parse response
	var response SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("Search completed: total_hits=%d, returned_hits=%d", response.Hits.Total.Value, len(response.Hits.Hits))

	return &response, nil
}

// HealthCheck checks if OpenSearch is accessible
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.client.Info()
	return err
}
