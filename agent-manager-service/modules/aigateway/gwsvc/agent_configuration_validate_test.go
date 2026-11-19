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

package gwsvc

import (
	"context"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// fakeLLMProviderRepo implements LLMProviderRepository for unit tests; only GetByHandle is used.
type fakeLLMProviderRepo struct {
	gwrepo.LLMProviderRepository
	byHandle map[string]*gwmodels.LLMProvider
}

func (f *fakeLLMProviderRepo) GetByHandle(handle, _ string) (*gwmodels.LLMProvider, error) {
	if p, ok := f.byHandle[handle]; ok {
		return p, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func TestValidateProvidersInCatalog(t *testing.T) {
	s := &agentConfigurationService{
		llmProviderRepo: &fakeLLMProviderRepo{byHandle: map[string]*gwmodels.LLMProvider{
			"good":   {InCatalog: true},
			"notcat": {InCatalog: false},
		}},
	}
	ctx := context.Background()

	require.NoError(t, s.ValidateProvidersInCatalog(ctx, "org", []string{"good", "good"}),
		"valid handle (deduped) passes")
	require.ErrorIs(t, s.ValidateProvidersInCatalog(ctx, "org", []string{"missing"}),
		gwerr.ErrLLMProviderNotFound)
	require.ErrorIs(t, s.ValidateProvidersInCatalog(ctx, "org", []string{"notcat"}),
		httpx.ErrInvalidInput)
	require.ErrorIs(t, s.ValidateProvidersInCatalog(ctx, "org", []string{""}),
		httpx.ErrInvalidInput)
}
