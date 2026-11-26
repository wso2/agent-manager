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
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

func templateDoc(name string, spec gwspec.LlmProviderTemplateSpec) gwspec.LlmProviderTemplateDocument {
	return gwspec.LlmProviderTemplateDocument{
		ApiVersion: GroupVersion, Kind: KindLlmProviderTemplate,
		Metadata: gwspec.DocumentMetadata{Name: name}, Spec: spec,
	}
}

// A template document maps every field to the model and back to the document.
func TestLLMProviderTemplateDocumentRoundTrips(t *testing.T) {
	ei := func(id string) *gwspec.ExtractionIdentifier {
		return &gwspec.ExtractionIdentifier{Location: "payload", Identifier: id}
	}
	spec := gwspec.LlmProviderTemplateSpec{
		DisplayName: "OpenAI", Description: mapPtr("desc"),
		Metadata: &gwspec.LLMProviderTemplateMetadata{
			EndpointUrl: mapPtr("https://api"), LogoUrl: mapPtr("https://logo"), OpenapiSpecUrl: mapPtr("https://spec"),
			Auth: &gwspec.LLMProviderTemplateAuth{Type: mapPtr("apiKey"), Header: mapPtr("Authorization"), ValuePrefix: mapPtr("Bearer ")},
		},
		PromptTokens: ei("$.usage.prompt"), CompletionTokens: ei("$.usage.completion"), TotalTokens: ei("$.usage.total"),
		RemainingTokens: ei("$.remaining"), RequestModel: ei("$.model"), ResponseModel: ei("$.resp.model"),
	}

	model, err := LLMProviderTemplateFromDocument(templateDoc("openai", spec), "ou-1")
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, model.UUID)
	assert.Equal(t, "ou-1", model.OUID)
	assert.Equal(t, "openai", model.Handle)
	assert.Equal(t, "OpenAI", model.Name)
	assert.Equal(t, "Bearer ", model.Metadata.Auth.ValuePrefix)
	assert.Equal(t, "$.usage.total", model.TotalTokens.Identifier)

	model.CreatedBy = "alice"
	model.CreatedAt = time.Unix(100, 0)
	model.UpdatedAt = time.Unix(200, 0)
	doc := LLMProviderTemplateDocument(model)

	assert.Equal(t, GroupVersion, doc.ApiVersion)
	assert.Equal(t, KindLlmProviderTemplate, doc.Kind)
	assert.Equal(t, "openai", doc.Metadata.Name)
	assert.Equal(t, model.UUID.String(), *doc.Metadata.Uid)
	assert.Equal(t, "alice", *doc.Metadata.CreatedBy)
	assert.Equal(t, time.Unix(100, 0), *doc.Metadata.CreationTimestamp)
	assert.Equal(t, time.Unix(200, 0), *doc.Metadata.UpdateTimestamp)
	assert.Equal(t, spec, doc.Spec)
}

// A bare template has no metadata timestamps, optional blocks or extraction identifiers.
func TestLLMProviderTemplateDocumentWithoutOptionalFields(t *testing.T) {
	model, err := LLMProviderTemplateFromDocument(templateDoc("t", gwspec.LlmProviderTemplateSpec{
		DisplayName: "T", Metadata: &gwspec.LLMProviderTemplateMetadata{},
	}), "ou")
	require.NoError(t, err)
	require.NotNil(t, model.Metadata)
	assert.Nil(t, model.Metadata.Auth)
	assert.Nil(t, model.PromptTokens)

	doc := LLMProviderTemplateDocument(&gwmodels.LLMProviderTemplate{Handle: "t", Name: "T", Metadata: model.Metadata})

	assert.Nil(t, doc.Metadata.Uid)
	assert.Nil(t, doc.Metadata.CreatedBy)
	assert.Nil(t, doc.Metadata.CreationTimestamp)
	assert.Nil(t, doc.Metadata.UpdateTimestamp)
	assert.Nil(t, doc.Spec.Description)
	assert.Equal(t, &gwspec.LLMProviderTemplateMetadata{}, doc.Spec.Metadata)
	assert.Nil(t, doc.Spec.PromptTokens)
	assert.Nil(t, ConvertSpecToModelExtractionIdentifier(nil))
	assert.Nil(t, ConvertModelToSpecExtractionIdentifier(nil))
	assert.Nil(t, ConvertSpecToModelLLMProviderTemplateMetadata(nil))
	assert.Nil(t, ConvertModelToSpecLLMProviderTemplateMetadata(nil))
}

// A document the server will not accept names the field that is wrong.
func TestLLMProviderTemplateFromDocumentRefuses(t *testing.T) {
	valid := templateDoc("t", gwspec.LlmProviderTemplateSpec{DisplayName: "T"})
	cases := map[string]struct {
		mutate func(*gwspec.LlmProviderTemplateDocument)
		want   string
	}{
		"apiVersion":  {func(d *gwspec.LlmProviderTemplateDocument) { d.ApiVersion = "v9" }, "apiVersion"},
		"kind":        {func(d *gwspec.LlmProviderTemplateDocument) { d.Kind = KindLlmProvider }, "kind"},
		"name":        {func(d *gwspec.LlmProviderTemplateDocument) { d.Metadata.Name = " " }, "metadata.name"},
		"displayName": {func(d *gwspec.LlmProviderTemplateDocument) { d.Spec.DisplayName = "" }, "spec.displayName"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doc := valid
			tc.mutate(&doc)
			_, err := LLMProviderTemplateFromDocument(doc, "ou")
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrDocument))
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// The list envelope carries every template and the page bounds.
func TestLLMProviderTemplateList(t *testing.T) {
	list := LLMProviderTemplateList([]*gwmodels.LLMProviderTemplate{{Handle: "a", Name: "A"}, {Handle: "b", Name: "B"}}, 20, 0, 7)

	require.Len(t, list.List, 2)
	assert.Equal(t, int32(2), *list.Count)
	assert.Equal(t, "b", list.List[1].Metadata.Name)
	assert.Equal(t, int32(20), *list.Pagination.Limit)
	assert.Equal(t, int32(0), *list.Pagination.Offset)
	assert.Equal(t, int32(7), *list.Pagination.Total)
}
