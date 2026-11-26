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
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// KindMcpProxyScope is the kind of an MCP proxy scope document.
const KindMcpProxyScope = "McpProxyScope"

// MCPProxyScopeDocument renders one scope as its document. metadata.name is the action, which is
// immutable; spec.scope is the token scope string derived from it and the proxy's handle.
func MCPProxyScopeDocument(proxyHandle string, scope gwmodels.MCPProxyScope) gwspec.McpProxyScopeDocument {
	token := scope.ScopeString(proxyHandle)
	description := scope.Description
	tools := scope.Tools
	if tools == nil {
		tools = []string{}
	}

	meta := gwspec.DocumentMetadata{Name: scope.Action}
	if scope.UUID != uuid.Nil {
		uid := scope.UUID.String()
		meta.Uid = &uid
	}
	if !scope.CreatedAt.IsZero() {
		created := scope.CreatedAt
		meta.CreationTimestamp = &created
	}
	if !scope.UpdatedAt.IsZero() {
		updated := scope.UpdatedAt
		meta.UpdateTimestamp = &updated
	}

	return gwspec.McpProxyScopeDocument{
		ApiVersion: GroupVersion,
		Kind:       KindMcpProxyScope,
		Metadata:   meta,
		Spec: gwspec.McpProxyScopeSpec{
			Scope:       &token,
			Description: &description,
			Tools:       tools,
		},
	}
}

// MCPProxyScopeList renders a proxy's scopes as the list envelope. Scopes are not paginated, so
// the envelope carries no pagination block.
func MCPProxyScopeList(result *gwmodels.MCPProxyScopesResult) gwspec.MCPProxyScopeListResponse {
	if result == nil {
		count := int32(0)
		return gwspec.MCPProxyScopeListResponse{Count: &count, List: []gwspec.McpProxyScopeDocument{}}
	}

	docs := make([]gwspec.McpProxyScopeDocument, 0, len(result.Scopes))
	for i := range result.Scopes {
		docs = append(docs, MCPProxyScopeDocument(result.ProxyHandle, result.Scopes[i]))
	}

	count := int32(len(docs))
	return gwspec.MCPProxyScopeListResponse{Count: &count, List: docs}
}

// MCPProxyScopeFromDocument turns a posted document into the scope the service creates.
func MCPProxyScopeFromDocument(doc gwspec.McpProxyScopeDocument) (gwmodels.MCPProxyScopeInput, error) {
	if err := checkMCPProxyScopeDocument(doc); err != nil {
		return gwmodels.MCPProxyScopeInput{}, err
	}

	description := ""
	if doc.Spec.Description != nil {
		description = *doc.Spec.Description
	}
	return gwmodels.MCPProxyScopeInput{
		Action:      doc.Metadata.Name,
		Description: description,
		Tools:       doc.Spec.Tools,
	}, nil
}

// MCPProxyScopeUpdateFromDocument turns a PUT body into the service's update input. An absent
// spec key stays absent and is preserved, matching v1: a nil description or tool list means
// "leave unchanged", while a declared empty list unbinds the scope from every tool. The document
// is decoded once and its fields are carried over as they are, so nothing re-marshals an absent
// key into an empty one.
func MCPProxyScopeUpdateFromDocument(doc gwspec.McpProxyScopeDocument) (gwmodels.MCPProxyScopeUpdateInput, error) {
	if err := checkMCPProxyScopeDocument(doc); err != nil {
		return gwmodels.MCPProxyScopeUpdateInput{}, err
	}
	return gwmodels.MCPProxyScopeUpdateInput{
		Description: doc.Spec.Description,
		Tools:       doc.Spec.Tools,
	}, nil
}

// checkMCPProxyScopeDocument verifies the parts a Go type cannot.
func checkMCPProxyScopeDocument(doc gwspec.McpProxyScopeDocument) error {
	if doc.ApiVersion != GroupVersion {
		return fmt.Errorf("%w: apiVersion must be %q, got %q", ErrDocument, GroupVersion, doc.ApiVersion)
	}
	if doc.Kind != KindMcpProxyScope {
		return fmt.Errorf("%w: kind must be %q on this route, got %q", ErrDocument, KindMcpProxyScope, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return fmt.Errorf("%w: metadata.name is required - it is the scope's action", ErrDocument)
	}
	return nil
}
