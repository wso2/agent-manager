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
	"encoding/json"
	"fmt"
	"log"

	"github.com/opensearch-project/opensearch-go"
	"github.com/wso2-enterprise/agent-management-platform/traces-observer-service/config"
)

// Client wraps the OpenSearch client
type Client struct {
	client *opensearch.Client
	config *config.OpenSearchConfig
}

// NewClient creates a new OpenSearch client
func NewClient(cfg *config.OpenSearchConfig) (*Client, error) {
	opensearchConfig := opensearch.Config{
		Addresses: []string{cfg.Address},
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

// Search executes a search query
func (c *Client) Search(ctx context.Context, query map[string]interface{}) (*SearchResponse, error) {
	// Convert query to JSON
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	// Execute search
	res, err := c.client.Search(
		c.client.Search.WithContext(ctx),
		c.client.Search.WithIndex(c.config.Index),
		c.client.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search returned error: %s", res.Status())
	}

	// Parse response
	var response SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// HealthCheck checks if OpenSearch is accessible
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.client.Info()
	return err
}
