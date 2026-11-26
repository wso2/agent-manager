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

package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// maxDocumentBytes caps a posted document.
	maxDocumentBytes = 1 << 20
	// maxAgentListLimit is the largest page an agent list returns; high enough to list a whole org at once.
	maxAgentListLimit = 500
	// documentGroupVersion is the apiVersion of every document kind.
	documentGroupVersion = "ai-control-plane.choreolabs.com/v1"
	kindAgent            = "Agent"
)

var (
	// errDocument marks a document the server can name a bad field in; it answers 400 with that reason.
	errDocument = errors.New("invalid document")
	// errMalformedBody marks a body that is not JSON at all.
	errMalformedBody = errors.New("malformed body")
)

// requireProject reads the project query parameter, answering 400 when it is absent.
func requireProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	project := strings.TrimSpace(r.URL.Query().Get(utils.QueryParamProject))
	if project == "" {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "project query parameter is required")
		return "", false
	}
	return project, true
}

// writeDocumentError answers a body that is not a valid document with 400.
func writeDocumentError(w http.ResponseWriter, err error) {
	if errors.Is(err, errMalformedBody) {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	httpx.WriteErrorResponse(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), errDocument.Error()+": "))
}

// agentDocument renders a stored agent as a document.
func agentDocument(a *models.AgentResponse) spec.AgentDocument {
	doc := spec.AgentDocument{
		ApiVersion: documentGroupVersion,
		Kind:       kindAgent,
		Metadata: spec.ProjectScopedDocumentMetadata{
			Name:    a.Name,
			Project: a.ProjectName,
		},
		Spec: spec.AgentSpec{
			DisplayName:  a.DisplayName,
			Provisioning: spec.AgentProvisioning{Type: a.Provisioning.Type},
			AgentType:    spec.AgentType{Type: a.Type.Type},
		},
	}
	if a.UUID != "" {
		doc.Metadata.Uid = &a.UUID
	}
	if !a.CreatedAt.IsZero() {
		created := a.CreatedAt
		doc.Metadata.CreationTimestamp = &created
	}
	if a.CreatedBy != nil && a.CreatedBy.ID != "" {
		doc.Metadata.CreatedBy = &a.CreatedBy.ID
	}
	if a.Description != "" {
		doc.Spec.Description = &a.Description
	}
	if a.Type.SubType != "" {
		doc.Spec.AgentType.SubType = &a.Type.SubType
	}
	if len(a.Labels) > 0 {
		labels := a.Labels
		doc.Spec.Labels = &labels
	}
	if a.Status != "" {
		doc.Status = &spec.AgentStatus{State: &a.Status}
	}
	return doc
}

// agentList renders one page of agents in the list envelope.
func agentList(agents []*models.AgentResponse, limit, offset, total int32) spec.AgentListResponse {
	list := make([]spec.AgentDocument, 0, len(agents))
	for _, a := range agents {
		list = append(list, agentDocument(a))
	}
	return spec.AgentListResponse{
		Count: int32(len(list)),
		List:  list,
		Pagination: spec.Pagination{
			Limit:  &limit,
			Offset: &offset,
			Total:  &total,
		},
	}
}

// decodeAgentDocument decodes a posted Agent document and checks its envelope; it also returns the raw spec.
func decodeAgentDocument(body []byte) (spec.AgentDocument, map[string]json.RawMessage, error) {
	var doc spec.AgentDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) || !json.Valid(body) {
			return doc, nil, fmt.Errorf("%w: %w", errMalformedBody, err)
		}
		return doc, nil, fmt.Errorf("%w: %w", errDocument, err)
	}
	if doc.ApiVersion != documentGroupVersion {
		return doc, nil, fmt.Errorf("%w: apiVersion must be %q, got %q", errDocument, documentGroupVersion, doc.ApiVersion)
	}
	if doc.Kind != kindAgent {
		return doc, nil, fmt.Errorf("%w: kind must be %q on this route, got %q", errDocument, kindAgent, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return doc, nil, fmt.Errorf("%w: metadata.name is required - it is the agent's handle", errDocument)
	}
	if strings.TrimSpace(doc.Metadata.Project) == "" {
		return doc, nil, fmt.Errorf("%w: metadata.project is required", errDocument)
	}
	var envelope struct {
		Spec map[string]json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return doc, nil, fmt.Errorf("%w: %w", errDocument, err)
	}
	if envelope.Spec == nil {
		return doc, nil, fmt.Errorf("%w: spec is required", errDocument)
	}
	return doc, envelope.Spec, nil
}

// agentCreateFromDocument turns a POST body into the create request and the project it names.
func agentCreateFromDocument(body []byte) (spec.AgentDocument, spec.CreateAgentRequest, error) {
	doc, _, err := decodeAgentDocument(body)
	if err != nil {
		return doc, spec.CreateAgentRequest{}, err
	}
	agentType := doc.Spec.AgentType
	req := spec.CreateAgentRequest{
		Name:         doc.Metadata.Name,
		DisplayName:  doc.Spec.DisplayName,
		Description:  doc.Spec.Description,
		Provisioning: spec.Provisioning{Type: doc.Spec.Provisioning.Type},
		Labels:       doc.Spec.Labels,
		Extensions:   doc.Spec.Extensions,
	}
	if agentType.Type != "" {
		req.AgentType = &agentType
	}
	return doc, req, nil
}

// agentUpdateFromDocument turns a PUT body into the update request, keeping what it omits and refusing a change to a field fixed at creation.
func agentUpdateFromDocument(body []byte, stored *models.AgentResponse) (spec.UpdateAgentBasicInfoRequest, error) {
	doc, rawSpec, err := decodeAgentDocument(body)
	if err != nil {
		return spec.UpdateAgentBasicInfoRequest{}, err
	}
	if doc.Metadata.Name != stored.Name {
		return spec.UpdateAgentBasicInfoRequest{}, fmt.Errorf("%w: metadata.name does not match the agent in the path", errDocument)
	}
	if doc.Metadata.Project != stored.ProjectName {
		return spec.UpdateAgentBasicInfoRequest{}, fmt.Errorf("%w: metadata.project does not match the project of the agent", errDocument)
	}
	if _, ok := rawSpec["provisioning"]; ok && doc.Spec.Provisioning.Type != stored.Provisioning.Type {
		return spec.UpdateAgentBasicInfoRequest{}, fmt.Errorf("%w: spec.provisioning cannot be changed", errDocument)
	}
	// Only the type is compared: it is derived from provisioning, and no subtype is stored.
	if _, ok := rawSpec["agentType"]; ok && doc.Spec.AgentType.Type != stored.Type.Type {
		return spec.UpdateAgentBasicInfoRequest{}, fmt.Errorf("%w: spec.agentType cannot be changed", errDocument)
	}

	req := spec.UpdateAgentBasicInfoRequest{
		DisplayName: doc.Spec.DisplayName,
		Description: stored.Description,
		Labels:      doc.Spec.Labels,
		Extensions:  doc.Spec.Extensions,
	}
	if _, ok := rawSpec["description"]; ok {
		req.Description = doc.Spec.GetDescription()
	}
	return req, nil
}
