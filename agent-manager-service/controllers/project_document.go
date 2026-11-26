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
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

const kindProject = "Project"

// projectDocument renders a stored project as a document.
func projectDocument(p *models.ProjectResponse) spec.ProjectDocument {
	doc := spec.ProjectDocument{
		ApiVersion: documentGroupVersion,
		Kind:       kindProject,
		Metadata:   spec.DocumentMetadata{Name: p.Name},
		Spec:       spec.ProjectSpec{DisplayName: p.DisplayName},
	}
	if p.UUID != "" {
		doc.Metadata.Uid = &p.UUID
	}
	if !p.CreatedAt.IsZero() {
		created := p.CreatedAt
		doc.Metadata.CreationTimestamp = &created
	}
	if p.Description != "" {
		doc.Spec.Description = &p.Description
	}
	return doc
}

// projectList renders one page of projects in the list envelope.
func projectList(projects []*models.ProjectResponse, limit, offset, total int32) spec.ProjectListResponse {
	list := make([]spec.ProjectDocument, 0, len(projects))
	for _, p := range projects {
		if p != nil {
			list = append(list, projectDocument(p))
		}
	}
	return spec.ProjectListResponse{
		Count:      int32(len(list)),
		List:       list,
		Pagination: spec.Pagination{Limit: &limit, Offset: &offset, Total: &total},
	}
}

// decodeProjectDocument decodes a posted Project document and checks its envelope; it also returns the raw spec.
func decodeProjectDocument(body []byte) (spec.ProjectDocument, map[string]json.RawMessage, error) {
	var doc spec.ProjectDocument
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
	if doc.Kind != kindProject {
		return doc, nil, fmt.Errorf("%w: kind must be %q on this route, got %q", errDocument, kindProject, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return doc, nil, fmt.Errorf("%w: metadata.name is required - it is the project's handle", errDocument)
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

// projectCreateFromDocument turns a POST body into the create request.
func projectCreateFromDocument(body []byte) (spec.CreateProjectRequest, error) {
	doc, _, err := decodeProjectDocument(body)
	if err != nil {
		return spec.CreateProjectRequest{}, err
	}
	return spec.CreateProjectRequest{
		Name:        doc.Metadata.Name,
		DisplayName: doc.Spec.DisplayName,
		Description: doc.Spec.Description,
	}, nil
}

// projectUpdateFromDocument turns a PUT body into the update request, keeping a description it omits.
func projectUpdateFromDocument(body []byte, stored *models.ProjectResponse) (spec.UpdateProjectRequest, error) {
	doc, rawSpec, err := decodeProjectDocument(body)
	if err != nil {
		return spec.UpdateProjectRequest{}, err
	}
	if doc.Metadata.Name != stored.Name {
		return spec.UpdateProjectRequest{}, fmt.Errorf("%w: metadata.name does not match the project in the path", errDocument)
	}
	req := spec.UpdateProjectRequest{DisplayName: doc.Spec.DisplayName, Description: stored.Description}
	if _, ok := rawSpec["description"]; ok {
		req.Description = doc.Spec.GetDescription()
	}
	return req, nil
}
