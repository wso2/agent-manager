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

package gwconv

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// KindMcpProxy is the kind of an MCP proxy document.
const KindMcpProxy = "McpProxy"

// MCPProxyDocument renders an MCP proxy as its document.
func MCPProxyDocument(dto *gwmodels.MCPProxyDTO) (gwspec.McpProxyDocument, error) {
	if dto == nil {
		return gwspec.McpProxyDocument{}, errors.New("no MCP proxy to render")
	}
	if strings.TrimSpace(dto.ID) == "" {
		return gwspec.McpProxyDocument{}, errors.New("MCP proxy has no handle: cannot name the document")
	}

	spec, err := mcpProxySpecOf(dto)
	if err != nil {
		return gwspec.McpProxyDocument{}, err
	}
	spec.DisplayName = dto.Name

	meta := gwspec.DocumentMetadata{Name: dto.ID}
	meta.CreatedBy = dto.CreatedBy
	meta.CreationTimestamp = dto.CreatedAt
	meta.UpdateTimestamp = dto.UpdatedAt

	return gwspec.McpProxyDocument{
		ApiVersion: GroupVersion,
		Kind:       KindMcpProxy,
		Metadata:   meta,
		Spec:       spec,
	}, nil
}

// mcpProxySpecOf bridges the flat body into the document spec through JSON: both are generated
// from the same schemas, so a hand-written mapping of the nested endpoint types would be chances
// to drop a field silently. The renamed and relocated fields are set by the caller.
func mcpProxySpecOf(v any) (gwspec.McpProxySpec, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return gwspec.McpProxySpec{}, fmt.Errorf("marshal the MCP proxy: %w", err)
	}
	var spec gwspec.McpProxySpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return gwspec.McpProxySpec{}, fmt.Errorf("unmarshal the MCP proxy into its document spec: %w", err)
	}
	return spec, nil
}

// MCPProxyList renders a page of MCP proxies as the list envelope. Each row is a full document
// rather than a summary type, sparse only in the blocks the list query does not read - endpoints
// above all, which a list view never renders.
func MCPProxyList(resp *gwmodels.MCPProxyListResponse) (gwspec.MCPProxyListResponse, error) {
	if resp == nil {
		return gwspec.MCPProxyListResponse{}, errors.New("no MCP proxy page to render")
	}

	docs := make([]gwspec.McpProxyDocument, 0, len(resp.List))
	for i := range resp.List {
		doc, err := mcpProxyListItemDocument(resp.List[i])
		if err != nil {
			return gwspec.MCPProxyListResponse{}, err
		}
		docs = append(docs, doc)
	}

	count := int32(len(docs))
	// v1's pagination.count is the collection total; limit and offset are the page itself.
	limit, offset, total := int32(resp.Pagination.Limit), int32(resp.Pagination.Offset), int32(resp.Pagination.Count)
	return gwspec.MCPProxyListResponse{
		Count:      &count,
		List:       docs,
		Pagination: &gwspec.Pagination{Limit: &limit, Offset: &offset, Total: &total},
	}, nil
}

// mcpProxyListItemDocument renders one list row as a document.
func mcpProxyListItemDocument(item gwmodels.MCPProxyListItem) (gwspec.McpProxyDocument, error) {
	if item.ID == nil || strings.TrimSpace(*item.ID) == "" {
		return gwspec.McpProxyDocument{}, errors.New("MCP proxy has no handle: cannot name the document")
	}

	spec, err := mcpProxySpecOf(item)
	if err != nil {
		return gwspec.McpProxyDocument{}, err
	}
	if item.Name != nil {
		spec.DisplayName = *item.Name
	}

	meta := gwspec.DocumentMetadata{Name: *item.ID}
	meta.CreatedBy = item.CreatedBy
	meta.CreationTimestamp = item.CreatedAt
	meta.UpdateTimestamp = item.UpdatedAt

	return gwspec.McpProxyDocument{
		ApiVersion: GroupVersion,
		Kind:       KindMcpProxy,
		Metadata:   meta,
		Spec:       spec,
	}, nil
}

// MCPProxyFromDocument turns a posted document into the body the MCP proxy service takes, so a
// document runs v1's create and update paths unchanged.
//
// It takes the raw body rather than a decoded document because the struct cannot answer whether a
// key was sent: spec.endpoints is `omitempty`, so re-marshalling a decoded spec renders an
// explicit [] identically to an absent key. Decoding the raw spec straight into the flat body
// keeps the two apart.
func MCPProxyFromDocument(body []byte) (gwspec.McpProxyDocument, *gwmodels.MCPProxyDTO, error) {
	var doc gwspec.McpProxyDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return doc, nil, fmt.Errorf("%w: %w", ErrMalformedBody, err)
	}
	if err := checkMCPProxyDocument(doc); err != nil {
		return doc, nil, err
	}

	rawSpec, err := rawSpecOf(body)
	if err != nil {
		return doc, nil, err
	}
	var req gwmodels.MCPProxyDTO
	if err := json.Unmarshal(rawSpec, &req); err != nil {
		return doc, nil, fmt.Errorf("%w: %w", ErrDocument, err)
	}

	// The §5.2 rename in reverse: handle from metadata.name, label from spec.displayName.
	req.ID = doc.Metadata.Name
	req.Name = doc.Spec.DisplayName

	return doc, &req, nil
}

// checkMCPProxyDocument verifies the parts a Go type cannot: a kind that does not match the route
// would otherwise be built out of whatever fields happened to line up.
func checkMCPProxyDocument(doc gwspec.McpProxyDocument) error {
	if doc.ApiVersion != GroupVersion {
		return fmt.Errorf("%w: apiVersion must be %q, got %q", ErrDocument, GroupVersion, doc.ApiVersion)
	}
	if doc.Kind != KindMcpProxy {
		return fmt.Errorf("%w: kind must be %q on this route, got %q", ErrDocument, KindMcpProxy, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return fmt.Errorf("%w: metadata.name is required - it is the MCP proxy's handle", ErrDocument)
	}
	if strings.TrimSpace(doc.Spec.DisplayName) == "" {
		return fmt.Errorf("%w: spec.displayName is required", ErrDocument)
	}
	return nil
}
