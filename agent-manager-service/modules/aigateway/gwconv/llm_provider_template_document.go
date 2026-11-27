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

// KindLlmProviderTemplate is the kind of an LLM provider template document.
const KindLlmProviderTemplate = "LlmProviderTemplate"

// LLMProviderTemplateDocument renders a template as its document.
func LLMProviderTemplateDocument(model *gwmodels.LLMProviderTemplate) gwspec.LlmProviderTemplateDocument {
	meta := gwspec.DocumentMetadata{Name: model.Handle}
	if model.UUID != uuid.Nil {
		uid := model.UUID.String()
		meta.Uid = &uid
	}
	if !model.CreatedAt.IsZero() {
		created := model.CreatedAt
		meta.CreationTimestamp = &created
	}
	if !model.UpdatedAt.IsZero() {
		updated := model.UpdatedAt
		meta.UpdateTimestamp = &updated
	}
	if model.CreatedBy != "" {
		by := model.CreatedBy
		meta.CreatedBy = &by
	}

	return gwspec.LlmProviderTemplateDocument{
		ApiVersion: GroupVersion,
		Kind:       KindLlmProviderTemplate,
		Metadata:   meta,
		Spec: gwspec.LlmProviderTemplateSpec{
			DisplayName:      model.Name,
			Description:      stringToPtr(model.Description),
			Metadata:         ConvertModelToSpecLLMProviderTemplateMetadata(model.Metadata),
			PromptTokens:     ConvertModelToSpecExtractionIdentifier(model.PromptTokens),
			CompletionTokens: ConvertModelToSpecExtractionIdentifier(model.CompletionTokens),
			TotalTokens:      ConvertModelToSpecExtractionIdentifier(model.TotalTokens),
			RemainingTokens:  ConvertModelToSpecExtractionIdentifier(model.RemainingTokens),
			RequestModel:     ConvertModelToSpecExtractionIdentifier(model.RequestModel),
			ResponseModel:    ConvertModelToSpecExtractionIdentifier(model.ResponseModel),
		},
	}
}

// LLMProviderTemplateList renders a page of templates as the list envelope.
func LLMProviderTemplateList(models []*gwmodels.LLMProviderTemplate, limit, offset, total int) gwspec.LLMProviderTemplateListResponse {
	docs := make([]gwspec.LlmProviderTemplateDocument, 0, len(models))
	for _, m := range models {
		docs = append(docs, LLMProviderTemplateDocument(m))
	}
	count := int32(len(docs))
	lim, off, tot := int32(limit), int32(offset), int32(total)
	return gwspec.LLMProviderTemplateListResponse{
		Count:      &count,
		List:       docs,
		Pagination: &gwspec.Pagination{Limit: &lim, Offset: &off, Total: &tot},
	}
}

// LLMProviderTemplateFromDocument turns a posted or put document into the template the service stores.
func LLMProviderTemplateFromDocument(doc gwspec.LlmProviderTemplateDocument, ouID string) (*gwmodels.LLMProviderTemplate, error) {
	if doc.ApiVersion != GroupVersion {
		return nil, fmt.Errorf("%w: apiVersion must be %q, got %q", ErrDocument, GroupVersion, doc.ApiVersion)
	}
	if doc.Kind != KindLlmProviderTemplate {
		return nil, fmt.Errorf("%w: kind must be %q on this route, got %q", ErrDocument, KindLlmProviderTemplate, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return nil, fmt.Errorf("%w: metadata.name is required - it is the template's handle", ErrDocument)
	}
	if strings.TrimSpace(doc.Spec.DisplayName) == "" {
		return nil, fmt.Errorf("%w: spec.displayName is required", ErrDocument)
	}

	s := doc.Spec
	return &gwmodels.LLMProviderTemplate{
		UUID:             uuid.New(),
		OUID:             ouID,
		Handle:           doc.Metadata.Name,
		Name:             s.DisplayName,
		Description:      ptrToString(s.Description),
		Metadata:         ConvertSpecToModelLLMProviderTemplateMetadata(s.Metadata),
		PromptTokens:     ConvertSpecToModelExtractionIdentifier(s.PromptTokens),
		CompletionTokens: ConvertSpecToModelExtractionIdentifier(s.CompletionTokens),
		TotalTokens:      ConvertSpecToModelExtractionIdentifier(s.TotalTokens),
		RemainingTokens:  ConvertSpecToModelExtractionIdentifier(s.RemainingTokens),
		RequestModel:     ConvertSpecToModelExtractionIdentifier(s.RequestModel),
		ResponseModel:    ConvertSpecToModelExtractionIdentifier(s.ResponseModel),
	}, nil
}
