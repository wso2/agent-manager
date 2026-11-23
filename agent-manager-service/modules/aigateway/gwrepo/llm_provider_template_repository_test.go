//go:build integration

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

package gwrepo

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// TestLLMProviderTemplateRepo_UpdateKeepsTemplateReadable guards against an
// update storing the template's already-encoded configuration as a JSON
// string, after which every read of the template, and the org's template
// list, failed to decode it.
func TestLLMProviderTemplateRepo_UpdateKeepsTemplateReadable(t *testing.T) {
	ou := newOrg(t)
	repo := NewLLMProviderTemplateRepo(db.GetDB())

	require.NoError(t, repo.Create(&gwmodels.LLMProviderTemplate{
		OUID: ou, Handle: "custom-tpl", Name: "Custom",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{EndpointURL: "https://llm.example.test/v1"},
	}))
	require.NoError(t, repo.Update(&gwmodels.LLMProviderTemplate{
		OUID: ou, Handle: "custom-tpl", Name: "Custom", Description: "updated",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{EndpointURL: "https://llm.example.test/v2"},
	}))

	got, err := repo.GetByHandle("custom-tpl", ou)
	require.NoError(t, err)
	require.Equal(t, "updated", got.Description)
	require.NotNil(t, got.Metadata)
	require.Equal(t, "https://llm.example.test/v2", got.Metadata.EndpointURL)
}
