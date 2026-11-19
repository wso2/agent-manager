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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

// A provider mid-delete (LLMProviderService.Delete has claimed it via MarkDeleting)
// must reject new proxies. The authoritative check lives in LLMProxyRepo.Create,
// which locks and rechecks the provider's status inside the same transaction as the
// insert (see IsDeletingForUpdate) — this test only pins down that
// LLMProxyService.Create propagates that rejection rather than wrapping it into an
// opaque error (issue #1739 follow-up: TOCTOU between the proxies check and the
// delete completing).
func TestLLMProxyService_Create_PropagatesProviderBeingDeletedFromRepo(t *testing.T) {
	providerUUID := uuid.New()
	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(_, _ string) (*gwmodels.LLMProvider, error) {
			return &gwmodels.LLMProvider{UUID: providerUUID}, nil
		},
	}
	proxyRepo := &gwrepomocks.LLMProxyRepositoryMock{
		ExistsFunc: func(_, _ string) (bool, error) { return false, nil },
		CreateFunc: func(_ context.Context, _ *gwmodels.LLMProxy, _, _, _, _ string) error {
			return gwerr.ErrLLMProviderBeingDeleted
		},
	}
	svc := NewLLMProxyService(proxyRepo, providerRepo)

	proxy := &gwmodels.LLMProxy{
		ProjectUUID: uuid.New(),
		Configuration: gwmodels.LLMProxyConfig{
			Name:     "my-proxy",
			Version:  "v1",
			Provider: providerUUID.String(),
		},
	}

	_, err := svc.Create(context.Background(), "ou-acme", "creator", proxy)

	require.Error(t, err)
	assert.ErrorIs(t, err, gwerr.ErrLLMProviderBeingDeleted)
}
