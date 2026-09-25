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

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestMonitorTemperatureValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      string
		optional  bool
		config    map[string]interface{}
		wantError bool
	}{
		{"anthropic_missing", "llm_judge", true, map[string]interface{}{}, false},
		{"other_provider_missing", "llm_judge", false, map[string]interface{}{}, true},
		{"code_parameter_required", "code", true, map[string]interface{}{}, true},
		{"saved_value_preserved", "llm_judge", true, map[string]interface{}{"temperature": 0.7}, false},
		{"invalid_value_rejected", "llm_judge", true, map[string]interface{}{"temperature": "invalid"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := []models.EvaluatorConfigParam{{Key: "temperature", Type: "float", Required: true}}
			cust := &repomocks.CustomEvaluatorRepositoryMock{
				GetByIdentifierFunc: func(ouID, identifier string) (*models.CustomEvaluator, error) {
					assert.Equal(t, "org-1", ouID)
					return &models.CustomEvaluator{Identifier: identifier, Type: tc.kind, ConfigSchema: schema}, nil
				},
			}
			evaluators := NewEvaluatorManagerService(discardLogger(), cust, nil)
			svc := NewMonitorManagerService(discardLogger(), nil, nil, nil, nil, evaluators, nil, nil, nil, nil, nil).(*monitorManagerService)
			configs := []models.MonitorEvaluator{{Identifier: "custom-policy-judge", DisplayName: "Judge", Config: tc.config}}
			_, err := svc.validateEvaluators(context.Background(), "org-1", configs, func() (bool, error) {
				return tc.optional, nil
			})
			if tc.wantError {
				require.ErrorIs(t, err, utils.ErrInvalidInput)
			} else {
				require.NoError(t, err)
			}
			assert.True(t, schema[0].Required, "catalog schema must not be modified")
			assert.Equal(t, tc.config, configs[0].Config)
		})
	}
}

func TestMonitorTemperaturePolicyLookupError(t *testing.T) {
	lookupErr := errors.New("provider lookup unavailable")
	cust := &repomocks.CustomEvaluatorRepositoryMock{
		GetByIdentifierFunc: func(_, identifier string) (*models.CustomEvaluator, error) {
			return &models.CustomEvaluator{Identifier: identifier, Type: "llm_judge", ConfigSchema: []models.EvaluatorConfigParam{
				{Key: "temperature", Type: "float", Required: true},
			}}, nil
		},
	}
	evaluators := NewEvaluatorManagerService(discardLogger(), cust, nil)
	svc := NewMonitorManagerService(discardLogger(), nil, nil, nil, nil, evaluators, nil, nil, nil, nil, nil).(*monitorManagerService)
	_, err := svc.validateEvaluators(context.Background(), "org-1", []models.MonitorEvaluator{{Identifier: "custom-policy-judge"}}, func() (bool, error) {
		return false, lookupErr
	})
	require.ErrorIs(t, err, lookupErr)
	assert.NotErrorIs(t, err, utils.ErrInvalidInput)
}

func TestMonitorTemperatureEffectiveProvider(t *testing.T) {
	providerID := uuid.New()
	monitorID := uuid.New()
	providers := &repomocks.LLMProviderRepositoryMock{
		GetByHandleFunc: func(handle, ouID string) (*models.LLMProvider, error) {
			assert.Equal(t, "org-1", ouID)
			return &models.LLMProvider{TemplateHandle: handle}, nil
		},
		GetByUUIDFunc: func(id, ouID string) (*models.LLMProvider, error) {
			assert.Equal(t, providerID.String(), id)
			assert.Equal(t, "org-1", ouID)
			return &models.LLMProvider{TemplateHandle: "anthropic"}, nil
		},
	}
	mappings := &repomocks.MonitorLLMMappingRepositoryMock{
		ListByMonitorIDFunc: func(_ context.Context, id uuid.UUID) ([]models.MonitorLLMMapping, error) {
			assert.Equal(t, monitorID, id)
			return []models.MonitorLLMMapping{{LLMProxy: &models.LLMProxy{ProviderUUID: providerID}}}, nil
		},
	}
	provisioner := NewLLMProxyProvisioner(discardLogger(), providers, nil, nil, nil, nil, nil, nil, nil)
	svc := NewMonitorManagerService(discardLogger(), nil, nil, nil, nil, nil, nil, nil, provisioner, mappings, nil).(*monitorManagerService)

	optional, err := svc.monitorTemperatureOptional(context.Background(), "org-1", nil, monitorID)
	require.NoError(t, err)
	assert.True(t, optional, "PATCH without a provider uses the stored provider")

	optional, err = svc.monitorTemperatureOptional(context.Background(), "org-1", &models.MonitorLLMProviderRef{ProviderName: "openai"}, monitorID)
	require.NoError(t, err)
	assert.False(t, optional, "switching providers restores required-temperature validation")

	optional, err = svc.monitorTemperatureOptional(context.Background(), "org-1", &models.MonitorLLMProviderRef{ProviderName: "anthropic"}, uuid.Nil)
	require.NoError(t, err)
	assert.True(t, optional, "create uses the requested provider")
}
