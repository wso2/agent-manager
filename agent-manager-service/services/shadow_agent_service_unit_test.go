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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	shadowTestOU     = "ou-1"
	shadowSecretKey  = "sk-admin-SECRETSECRETSECRET-wxyz"
	shadowPromptMark = "SECRET-PROMPT-MARKER"
)

var shadowTestKey = bytes.Repeat([]byte{7}, 32)

func newShadowSvc(repo *repomocks.ShadowAgentRepositoryMock) *shadowAgentService {
	return NewShadowAgentService(repo, ShadowConnectors{}, shadowTestKey, discardLogger()).(*shadowAgentService)
}

const shadowProjectKey = "sk-proj-AAAAAAAAAAAAAAAA"

// openAISettings is a valid OpenAI form: a project key.
func openAISettings() map[string]any {
	return map[string]any{"scopedKeys": map[string]any{"proj_a": shadowProjectKey}}
}

func openAIInput() ShadowConnectionInput {
	return ShadowConnectionInput{Name: "OpenAI prod", Provider: models.ShadowProviderOpenAI, Settings: openAISettings()}
}

// ---- connections ----

func TestShadowCreateConnection_EncryptsSecretAndReturnsOnlyHints(t *testing.T) {
	var stored *models.ShadowConnection
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{}, nil
		},
		CreateConnectionFunc: func(_ context.Context, c *models.ShadowConnection) error {
			stored = c
			return nil
		},
	}
	svc := newShadowSvc(repo)
	v, err := svc.CreateConnection(auditableCtx(t), shadowTestOU, openAIInput())
	require.NoError(t, err)
	require.NotNil(t, stored)

	assert.Equal(t, shadowTestOU, stored.OUID)
	assert.NotContains(t, string(stored.SecretEncrypted), "SECRETSECRET", "the secret is stored encrypted")
	secret, err := decryptShadowSecret(stored.SecretEncrypted, shadowTestKey)
	require.NoError(t, err)
	assert.Equal(t, shadowProjectKey, secret.ScopedKeys["proj_a"])
	assert.Equal(t, []string{"proj_a"}, stored.Config.ScopedKeyIDs)
	assert.Equal(t, map[string]string{"scopedKeys.proj_a": "sk-p…AAAA"}, stored.SecretHints)
	assert.Equal(t, "https://api.openai.com/v1", stored.Config.Endpoints[sa.EPOpenAIAPI])

	view, err := json.Marshal(v.Settings)
	require.NoError(t, err)
	assert.NotContains(t, string(view), "SECRETSECRET", "settings never carry a secret")
	assert.Empty(t, v.EndpointWarnings)
}

func TestShadowCreateConnection_RefusesWithoutAuditRecorder(t *testing.T) {
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{}, nil
		},
		// CreateConnectionFunc nil: must not be reached.
	}
	_, err := newShadowSvc(repo).CreateConnection(context.Background(), shadowTestOU, openAIInput())
	assert.ErrorIs(t, err, audit.ErrRecorderUnavailable)
}

func TestShadowCreateConnection_Validation(t *testing.T) {
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{}, nil
		},
	}
	svc := newShadowSvc(repo)
	tests := map[string]ShadowConnectionInput{
		"unknown provider":        {Name: "x", Provider: "acme", Settings: map[string]any{}},
		"missing name":            {Name: " ", Provider: models.ShadowProviderOpenAI, Settings: openAISettings()},
		"missing required secret": {Name: "x", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{}},
		"unknown setting": {Name: "x", Provider: models.ShadowProviderOpenAI, Settings: func() map[string]any {
			s := openAISettings()
			s["region"] = "x"
			return s
		}()},
		"removed setting (deep inspection)": {Name: "x", Provider: models.ShadowProviderOpenAI, Settings: func() map[string]any {
			s := openAISettings()
			s["deepInspection"] = true
			return s
		}()},
		"unknown endpoint": {
			Name: "x", Provider: models.ShadowProviderOpenAI, Settings: openAISettings(),
			Endpoints: map[string]string{"nope": "https://x"},
		},
		"removed endpoint (CloudTrail)": {
			Name: "x", Provider: models.ShadowProviderBedrock,
			Settings:  map[string]any{"regions": []any{"us-east-1"}, "accessKeyId": "AKIA", "secretAccessKey": "s"},
			Endpoints: map[string]string{"cloudtrail": "https://cloudtrail.us-east-1.amazonaws.com"},
		},
		"endpoint with a query": {
			Name: "x", Provider: models.ShadowProviderOpenAI, Settings: openAISettings(),
			Endpoints: map[string]string{sa.EPOpenAIAPI: "https://api.openai.com/v1?key=1"},
		},
		"wrong type": {Name: "x", Provider: models.ShadowProviderBedrock, Settings: map[string]any{
			"regions": "us-east-1", "accessKeyId": "AKIA", "secretAccessKey": "s",
		}},
		"missing required list": {Name: "x", Provider: models.ShadowProviderGoogle, Settings: map[string]any{
			"projectIds": []any{}, "locations": []any{"us-central1"}, "serviceAccountJson": `{"type":"service_account"}`,
		}},
		"service account key is not JSON": {Name: "x", Provider: models.ShadowProviderGoogle, Settings: map[string]any{
			"projectIds": []any{"p"}, "locations": []any{"us-central1"}, "serviceAccountJson": "not json",
		}},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.CreateConnection(auditableCtx(t), shadowTestOU, in)
			assert.ErrorIs(t, err, utils.ErrInvalidInput)
		})
	}
}

func TestShadowCreateConnection_CustomEndpointWarnsButIsAllowed(t *testing.T) {
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{}, nil
		},
		CreateConnectionFunc: func(context.Context, *models.ShadowConnection) error { return nil },
	}
	in := openAIInput()
	in.Endpoints = map[string]string{sa.EPOpenAIAPI: "http://localhost:18080/v1/"}
	v, err := newShadowSvc(repo).CreateConnection(auditableCtx(t), shadowTestOU, in)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:18080/v1", v.Connection.Config.Endpoints[sa.EPOpenAIAPI])
	require.Len(t, v.EndpointWarnings, 1)
	assert.Contains(t, v.EndpointWarnings[0], "not an official OpenAI host")
}

func TestShadowCreateConnection_AzureCheckboxDefaultsAndOptionalFields(t *testing.T) {
	var stored *models.ShadowConnection
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{}, nil
		},
		CreateConnectionFunc: func(_ context.Context, c *models.ShadowConnection) error {
			stored = c
			return nil
		},
	}
	_, err := newShadowSvc(repo).CreateConnection(auditableCtx(t), shadowTestOU, ShadowConnectionInput{
		Name: "Foundry", Provider: models.ShadowProviderFoundry,
		Settings: map[string]any{"tenantId": "t", "clientId": "c", "clientSecret": "s3cr3t-value-123456"},
	})
	require.NoError(t, err)
	assert.True(t, stored.Config.AllSubscriptions, "ticked by default")
	assert.True(t, stored.Config.IncludeClassicAgents, "ticked by default")
}

func TestShadowUpdateConnection_EmptySecretKeepsStoredOne(t *testing.T) {
	ctx := auditableCtx(t)
	encrypted, err := (&shadowAgentService{encryptionKey: shadowTestKey}).encryptSecret(models.ShadowConnectionSecret{
		ScopedKeys: map[string]string{"proj_a": "sk-proj-AAAAAAAAAAAAAAAA"},
	})
	require.NoError(t, err)
	id := uuid.New()
	existing := &models.ShadowConnection{
		ID: id, OUID: shadowTestOU, Name: "OpenAI", Provider: models.ShadowProviderOpenAI,
		Config:          models.ShadowConnectionConfig{Endpoints: sa.DefaultEndpoints(models.ShadowProviderOpenAI)},
		SecretEncrypted: encrypted,
	}
	var updated *models.ShadowConnection
	repo := &repomocks.ShadowAgentRepositoryMock{
		GetConnectionFunc: func(context.Context, string, uuid.UUID) (*models.ShadowConnection, error) {
			c := *existing
			return &c, nil
		},
		UpdateConnectionFunc: func(_ context.Context, c *models.ShadowConnection) error {
			updated = c
			return nil
		},
	}
	svc := newShadowSvc(repo)
	_, err = svc.UpdateConnection(ctx, shadowTestOU, id, ShadowConnectionInput{
		Name: "OpenAI renamed", Provider: models.ShadowProviderOpenAI,
		// An empty value for a known project key keeps the stored key.
		Settings: map[string]any{"scopedKeys": map[string]any{"proj_a": ""}},
	})
	require.NoError(t, err)
	secret, err := decryptShadowSecret(updated.SecretEncrypted, shadowTestKey)
	require.NoError(t, err)
	assert.Equal(t, "sk-proj-AAAAAAAAAAAAAAAA", secret.ScopedKeys["proj_a"])
	assert.Equal(t, []string{"proj_a"}, updated.Config.ScopedKeyIDs)

	_, err = svc.UpdateConnection(ctx, shadowTestOU, id, ShadowConnectionInput{
		Name: "x", Provider: models.ShadowProviderAnthropic, Settings: map[string]any{},
	})
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "the provider cannot change")
}

// TestShadowScopedKeysRequired: OpenAI and Anthropic list agents with
// project or workspace keys, so at least one is required. There is no Admin
// API key: it cannot list agents, and the form has no field for it.
func TestShadowScopedKeysRequired(t *testing.T) {
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{}, nil
		},
		CreateConnectionFunc: func(context.Context, *models.ShadowConnection) error { return nil },
	}
	svc := newShadowSvc(repo)
	ok := map[string]ShadowConnectionInput{
		"openai project keys only": {Name: "b", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{
			"scopedKeys": map[string]any{"proj_a": "sk-proj-a"},
		}},
		"anthropic workspace keys only": {Name: "c", Provider: models.ShadowProviderAnthropic, Settings: map[string]any{
			"scopedKeys": map[string]any{"wrkspc_a": "sk-ant-api03-a"},
		}},
	}
	for name, in := range ok {
		t.Run(name, func(t *testing.T) {
			_, err := svc.CreateConnection(auditableCtx(t), shadowTestOU, in)
			assert.NoError(t, err)
		})
	}
	bad := map[string]struct {
		in   ShadowConnectionInput
		want string
	}{
		"openai admin key": {ShadowConnectionInput{Name: "a", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{
			"apiKey": "sk-admin-x", "scopedKeys": map[string]any{"proj_a": "sk-proj-a"},
		}}, `unknown setting "apiKey" for OpenAI`},
		"openai empty key map": {ShadowConnectionInput{Name: "d", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{
			"scopedKeys": map[string]any{},
		}}, "Project keys needs at least one ID and key"},
		"anthropic admin key": {ShadowConnectionInput{Name: "e", Provider: models.ShadowProviderAnthropic, Settings: map[string]any{
			"apiKey": "sk-ant-admin01-x", "scopedKeys": map[string]any{"wrkspc_a": "sk-ant-api03-a"},
		}}, `unknown setting "apiKey" for Anthropic`},
		"anthropic no workspace key": {
			ShadowConnectionInput{Name: "g", Provider: models.ShadowProviderAnthropic, Settings: map[string]any{}},
			"Workspace keys needs at least one ID and key",
		},
		"removeSecrets on create": {ShadowConnectionInput{
			Name: "f", Provider: models.ShadowProviderOpenAI, Settings: openAISettings(), RemoveSecrets: []string{"scopedKeys"},
		}, "removeSecrets applies only when updating"},
	}
	for name, tc := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := svc.CreateConnection(auditableCtx(t), shadowTestOU, tc.in)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestShadowUpdateConnection_RemoveSecrets: only an optional secret field
// can be removed, the project keys stay required, and an Admin API key
// stored by an earlier version is dropped when the platform is saved.
func TestShadowUpdateConnection_RemoveSecrets(t *testing.T) {
	// stored takes the secret as the earlier JSON may have held it, so a
	// map can carry a member the current model no longer has.
	stored := func(t *testing.T, secret any, provider string) (*repomocks.ShadowAgentRepositoryMock, uuid.UUID, **models.ShadowConnection) {
		t.Helper()
		plain, err := json.Marshal(secret)
		require.NoError(t, err)
		encrypted, err := utils.EncryptBytes(plain, shadowTestKey)
		require.NoError(t, err)
		id := uuid.New()
		existing := &models.ShadowConnection{
			ID: id, OUID: shadowTestOU, Name: "c", Provider: provider,
			Config: models.ShadowConnectionConfig{Endpoints: sa.DefaultEndpoints(provider)}, SecretEncrypted: encrypted,
		}
		var updated *models.ShadowConnection
		repo := &repomocks.ShadowAgentRepositoryMock{
			GetConnectionFunc: func(context.Context, string, uuid.UUID) (*models.ShadowConnection, error) {
				c := *existing
				return &c, nil
			},
			UpdateConnectionFunc: func(_ context.Context, c *models.ShadowConnection) error {
				updated = c
				return nil
			},
		}
		return repo, id, &updated
	}
	both := models.ShadowConnectionSecret{ScopedKeys: map[string]string{"proj_a": "sk-proj-AAAAAAAAAAAAAAAA"}}
	legacy := map[string]any{"apiKey": shadowSecretKey, "scopedKeys": map[string]string{"proj_a": "sk-proj-AAAAAAAAAAAAAAAA"}}

	t.Run("an Admin key stored by an earlier version is dropped on save", func(t *testing.T) {
		repo, id, updated := stored(t, legacy, models.ShadowProviderOpenAI)
		v, err := newShadowSvc(repo).UpdateConnection(auditableCtx(t), shadowTestOU, id, ShadowConnectionInput{
			Name: "c", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{},
		})
		require.NoError(t, err)
		plain, err := utils.DecryptBytes((*updated).SecretEncrypted, shadowTestKey)
		require.NoError(t, err)
		assert.NotContains(t, string(plain), "apiKey")
		assert.NotContains(t, string(plain), "SECRETSECRET")
		secret, err := decryptShadowSecret((*updated).SecretEncrypted, shadowTestKey)
		require.NoError(t, err)
		assert.Equal(t, "sk-proj-AAAAAAAAAAAAAAAA", secret.ScopedKeys["proj_a"])
		assert.Equal(t, map[string]string{"scopedKeys.proj_a": "sk-p…AAAA"}, v.Connection.SecretHints)
	})
	t.Run("the Admin key cannot be removed: it is not a field", func(t *testing.T) {
		repo, id, _ := stored(t, legacy, models.ShadowProviderOpenAI)
		_, err := newShadowSvc(repo).UpdateConnection(auditableCtx(t), shadowTestOU, id, ShadowConnectionInput{
			Name: "c", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{}, RemoveSecrets: []string{"apiKey"},
		})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Contains(t, err.Error(), `"apiKey" is not a secret field of OpenAI`)
	})
	t.Run("a stored connection with only the Admin key must add a project key", func(t *testing.T) {
		repo, id, _ := stored(t, map[string]any{"apiKey": shadowSecretKey}, models.ShadowProviderOpenAI)
		_, err := newShadowSvc(repo).UpdateConnection(auditableCtx(t), shadowTestOU, id, ShadowConnectionInput{
			Name: "c", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{},
		})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Contains(t, err.Error(), "Project keys needs at least one ID and key")
		assert.Empty(t, repo.UpdateConnectionCalls())
	})
	t.Run("removing every project key is refused", func(t *testing.T) {
		repo, id, _ := stored(t, both, models.ShadowProviderOpenAI)
		_, err := newShadowSvc(repo).UpdateConnection(auditableCtx(t), shadowTestOU, id, ShadowConnectionInput{
			Name: "c", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{"scopedKeys": map[string]any{}},
		})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Empty(t, repo.UpdateConnectionCalls())
	})
	t.Run("a required secret cannot be removed", func(t *testing.T) {
		repo, id, _ := stored(t, models.ShadowConnectionSecret{ClientSecret: "s3cr3t-value-123456"}, models.ShadowProviderFoundry)
		_, err := newShadowSvc(repo).UpdateConnection(auditableCtx(t), shadowTestOU, id, ShadowConnectionInput{
			Name: "c", Provider: models.ShadowProviderFoundry,
			Settings:      map[string]any{"tenantId": "t", "clientId": "c"},
			RemoveSecrets: []string{"clientSecret"},
		})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Contains(t, err.Error(), "cannot be removed")
	})
	t.Run("not a secret field", func(t *testing.T) {
		repo, id, _ := stored(t, both, models.ShadowProviderOpenAI)
		for _, k := range []string{"scopedKeys", "deepInspection", "apiKey", "nope"} {
			_, err := newShadowSvc(repo).UpdateConnection(auditableCtx(t), shadowTestOU, id, ShadowConnectionInput{
				Name: "c", Provider: models.ShadowProviderOpenAI, Settings: map[string]any{}, RemoveSecrets: []string{k},
			})
			assert.ErrorIs(t, err, utils.ErrInvalidInput, k)
		}
	})
}

// TestShadowServiceOptions: unticked Bedrock/Google services are stored as
// DisabledServices and read back as unticked; a connection saved before the
// options existed has every service on; a Bedrock connection needs at least
// one service; the removed services are not options any more.
func TestShadowServiceOptions(t *testing.T) {
	var stored *models.ShadowConnection
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{}, nil
		},
		CreateConnectionFunc: func(_ context.Context, c *models.ShadowConnection) error {
			stored = c
			return nil
		},
	}
	svc := newShadowSvc(repo)
	bedrock := func(name string, extra map[string]any) ShadowConnectionInput {
		settings := map[string]any{"regions": []any{"us-east-1"}, "accessKeyId": "AKIA", "secretAccessKey": "s"}
		for k, v := range extra {
			settings[k] = v
		}
		return ShadowConnectionInput{Name: name, Provider: models.ShadowProviderBedrock, Settings: settings}
	}
	v, err := svc.CreateConnection(auditableCtx(t), shadowTestOU, bedrock("b1", map[string]any{sa.OptBedrockRegistry: false}))
	require.NoError(t, err)
	assert.Equal(t, []string{sa.OptBedrockRegistry}, stored.Config.DisabledServices)
	assert.Equal(t, false, v.Settings[sa.OptBedrockRegistry])
	assert.Equal(t, true, v.Settings[sa.OptBedrockAgentCore])
	assert.Equal(t, true, v.Settings[sa.OptBedrockClassicAgents])
	assert.Len(t, v.Settings, 7, "regions, access key ID, role, external ID and the three services; secrets are never settings")

	// Defaults: every service on.
	_, err = svc.CreateConnection(auditableCtx(t), shadowTestOU, bedrock("b2", nil))
	require.NoError(t, err)
	assert.Empty(t, stored.Config.DisabledServices)

	// A connection stored before the options existed reads as all on.
	legacy := shadowSettingsOf(models.ShadowProviderBedrock, models.ShadowConnectionConfig{Regions: []string{"us-east-1"}})
	for _, k := range []string{sa.OptBedrockAgentCore, sa.OptBedrockRegistry, sa.OptBedrockClassicAgents} {
		assert.Equal(t, true, legacy[k], k)
	}

	// The removed services are unknown settings.
	for _, k := range []string{"readCloudWatchMetrics", "readCloudTrail", "readInvocationLogging"} {
		_, err = svc.CreateConnection(auditableCtx(t), shadowTestOU, bedrock("b-"+k, map[string]any{k: false}))
		assert.ErrorIs(t, err, utils.ErrInvalidInput, k)
	}

	// Nothing selected: refused.
	_, err = svc.CreateConnection(auditableCtx(t), shadowTestOU, bedrock("b3", map[string]any{
		sa.OptBedrockAgentCore: false, sa.OptBedrockRegistry: false, sa.OptBedrockClassicAgents: false,
	}))
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "Select at least one Bedrock service to scan.")

	// Google: the engines are always listed, so the registry can be unticked.
	_, err = svc.CreateConnection(auditableCtx(t), shadowTestOU, ShadowConnectionInput{
		Name: "g", Provider: models.ShadowProviderGoogle, Settings: map[string]any{
			"projectIds": []any{"p"}, "locations": []any{"us-central1"}, "serviceAccountJson": `{"type":"service_account"}`,
			sa.OptGoogleRegistry: false,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{sa.OptGoogleRegistry}, stored.Config.DisabledServices)
	for _, k := range []string{"readMonitoring", "readAuditLogs"} {
		_, err = svc.CreateConnection(auditableCtx(t), shadowTestOU, ShadowConnectionInput{
			Name: "g-" + k, Provider: models.ShadowProviderGoogle, Settings: map[string]any{
				"projectIds": []any{"p"}, "locations": []any{"us-central1"}, "serviceAccountJson": `{"type":"service_account"}`,
				k: false,
			},
		})
		assert.ErrorIs(t, err, utils.ErrInvalidInput, k)
	}
}

func TestShadowGetConnection_NotFoundIsNotMasked(t *testing.T) {
	repo := &repomocks.ShadowAgentRepositoryMock{
		GetConnectionFunc: func(context.Context, string, uuid.UUID) (*models.ShadowConnection, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	_, err := newShadowSvc(repo).GetConnection(context.Background(), shadowTestOU, uuid.New())
	assert.ErrorIs(t, err, utils.ErrShadowConnectionNotFound)

	repo.GetConnectionFunc = func(context.Context, string, uuid.UUID) (*models.ShadowConnection, error) {
		return nil, assert.AnError
	}
	_, err = newShadowSvc(repo).GetConnection(context.Background(), shadowTestOU, uuid.New())
	assert.NotErrorIs(t, err, utils.ErrShadowConnectionNotFound)
	assert.ErrorIs(t, err, assert.AnError)
}

// ---- scans ----

func TestShadowCreateScan(t *testing.T) {
	c1 := models.ShadowConnection{ID: uuid.New()}
	c2 := models.ShadowConnection{ID: uuid.New()}
	var created *models.ShadowScan
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListConnectionsFunc: func(context.Context, string) ([]models.ShadowConnection, error) {
			return []models.ShadowConnection{c1, c2}, nil
		},
		CreateScanFunc: func(_ context.Context, s *models.ShadowScan) error {
			created = s
			return nil
		},
	}
	svc := newShadowSvc(repo)
	// All connections.
	scan, err := svc.CreateScan(auditableCtx(t), shadowTestOU, nil)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanQueued, created.Status)
	assert.Equal(t, []string{c1.ID.String(), c2.ID.String()}, created.ConnectionIDs)
	assert.Equal(t, created, scan)

	// One connection, and a repeated ID counted once.
	_, err = svc.CreateScan(auditableCtx(t), shadowTestOU, []uuid.UUID{c2.ID, c2.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{c2.ID.String()}, created.ConnectionIDs)

	_, err = svc.CreateScan(auditableCtx(t), shadowTestOU, []uuid.UUID{uuid.New()})
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "a connection of another org is not found")

	repo.CreateScanFunc = func(context.Context, *models.ShadowScan) error { return utils.ErrShadowScanInProgress }
	_, err = svc.CreateScan(auditableCtx(t), shadowTestOU, nil)
	assert.ErrorIs(t, err, utils.ErrShadowScanInProgress)

	repo.ListConnectionsFunc = func(context.Context, string) ([]models.ShadowConnection, error) {
		return []models.ShadowConnection{}, nil
	}
	_, err = svc.CreateScan(auditableCtx(t), shadowTestOU, nil)
	assert.ErrorIs(t, err, utils.ErrShadowNoConnections)
}

func TestShadowClearScans(t *testing.T) {
	repo := &repomocks.ShadowAgentRepositoryMock{
		DeleteFinishedScansFunc: func(context.Context, string) (int64, error) { return 3, nil },
	}
	n, err := newShadowSvc(repo).ClearScans(context.Background(), shadowTestOU)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	require.Len(t, repo.DeleteFinishedScansCalls(), 1)
	assert.Equal(t, shadowTestOU, repo.DeleteFinishedScansCalls()[0].OuID)

	repo.DeleteFinishedScansFunc = func(context.Context, string) (int64, error) { return 0, assert.AnError }
	_, err = newShadowSvc(repo).ClearScans(context.Background(), shadowTestOU)
	assert.ErrorIs(t, err, assert.AnError)
}

// ---- agents ----

func findingRepo(f models.ShadowFinding) *repomocks.ShadowAgentRepositoryMock {
	return &repomocks.ShadowAgentRepositoryMock{
		GetFindingFunc: func(context.Context, string, uuid.UUID) (*repositories.ShadowFindingRow, error) {
			return &repositories.ShadowFindingRow{ShadowFinding: f}, nil
		},
		ListSignalsFunc: func(context.Context, string, uuid.UUID) ([]models.ShadowFindingSignal, error) {
			return []models.ShadowFindingSignal{{Code: sa.SigOpenAIAgentObject, Source: "GET /v1/agents"}}, nil
		},
	}
}

func TestShadowGetFinding(t *testing.T) {
	other := models.ShadowFinding{ID: uuid.New(), DisplayName: "Orders agent", Provider: models.ShadowProviderBedrock}
	f := models.ShadowFinding{
		ID: uuid.New(), Provider: models.ShadowProviderFoundry, Kind: models.ShadowKindPlatformAgent,
		Related: []models.ShadowFindingRelation{
			{FindingID: other.ID.String(), Relation: "possible_duplicate"},
			{FindingID: uuid.New().String(), Relation: "possible_duplicate"}, // deleted with its connection
		},
	}
	repo := findingRepo(f)
	repo.GetFindingFunc = func(_ context.Context, _ string, id uuid.UUID) (*repositories.ShadowFindingRow, error) {
		switch id {
		case f.ID:
			return &repositories.ShadowFindingRow{ShadowFinding: f}, nil
		case other.ID:
			return &repositories.ShadowFindingRow{ShadowFinding: other}, nil
		}
		return nil, gorm.ErrRecordNotFound
	}
	v, err := newShadowSvc(repo).GetFinding(context.Background(), shadowTestOU, f.ID)
	require.NoError(t, err)
	require.Len(t, v.Signals, 1)
	require.Len(t, v.Related, 1, "a relation to a deleted agent is left out")
	assert.Equal(t, "Orders agent", v.Related[0].DisplayName)
	assert.Equal(t, models.ShadowProviderBedrock, v.Related[0].Provider)

	_, err = newShadowSvc(repo).GetFinding(context.Background(), shadowTestOU, uuid.New())
	assert.ErrorIs(t, err, utils.ErrShadowFindingNotFound)
}

func TestShadowListFindings_Validation(t *testing.T) {
	var got repositories.ShadowFindingFilter
	repo := &repomocks.ShadowAgentRepositoryMock{
		ListFindingsFunc: func(_ context.Context, _ string, f repositories.ShadowFindingFilter) ([]repositories.ShadowFindingRow, int, error) {
			got = f
			return nil, 0, nil
		},
	}
	svc := newShadowSvc(repo)
	_, _, err := svc.ListFindings(context.Background(), shadowTestOU, repositories.ShadowFindingFilter{Provider: "openai", Limit: 0, Offset: -3})
	require.NoError(t, err)
	assert.Equal(t, 50, got.Limit)
	assert.Equal(t, 0, got.Offset)
	for _, f := range []repositories.ShadowFindingFilter{
		{Provider: "acme"},
		{Status: "onboarded"},
		{Status: "retired"},
	} {
		_, _, err = svc.ListFindings(context.Background(), shadowTestOU, f)
		assert.ErrorIs(t, err, utils.ErrInvalidInput, f)
	}
}

func TestShadowUpdateFindingStatus(t *testing.T) {
	f := models.ShadowFinding{ID: uuid.New()}
	repo := findingRepo(f)
	repo.UpdateFindingStatusFunc = func(context.Context, string, uuid.UUID, string) error { return nil }
	svc := newShadowSvc(repo)
	for _, s := range []string{models.ShadowStatusReviewed, models.ShadowStatusDismissed, models.ShadowStatusNew} {
		_, err := svc.UpdateFindingStatus(auditableCtx(t), shadowTestOU, f.ID, s)
		require.NoError(t, err, s)
	}
	require.Len(t, repo.UpdateFindingStatusCalls(), 3)
	for _, s := range []string{"onboarded", "retired", "agent"} {
		_, err := svc.UpdateFindingStatus(auditableCtx(t), shadowTestOU, f.ID, s)
		assert.ErrorIs(t, err, utils.ErrInvalidInput, s)
	}
	repo.UpdateFindingStatusFunc = func(context.Context, string, uuid.UUID, string) error { return gorm.ErrRecordNotFound }
	_, err := svc.UpdateFindingStatus(auditableCtx(t), shadowTestOU, f.ID, models.ShadowStatusReviewed)
	assert.ErrorIs(t, err, utils.ErrShadowFindingNotFound)
}

func TestCorrelateShadowFindings(t *testing.T) {
	account := "/subscriptions/s/resourceGroups/rg/providers/Microsoft.CognitiveServices/accounts/acct"
	agent := models.ShadowFinding{
		ID: uuid.New(), Provider: models.ShadowProviderFoundry, Kind: models.ShadowKindPlatformAgent,
		DisplayName: "Support Bot", Attributes: map[string]any{"resourceId": account, "deployment": "gpt-4o", "agentKind": "prompt"},
	}
	external := models.ShadowFinding{
		ID: uuid.New(), Provider: models.ShadowProviderFoundry, Kind: models.ShadowKindPlatformAgent,
		DisplayName: "Orders-Agent", Attributes: map[string]any{"agentKind": "external"},
	}
	bedrock := models.ShadowFinding{ID: uuid.New(), Provider: models.ShadowProviderBedrock, Kind: models.ShadowKindRuntime, DisplayName: "orders agent"}
	unrelated := models.ShadowFinding{ID: uuid.New(), Provider: models.ShadowProviderGoogle, Kind: models.ShadowKindRuntime, DisplayName: "Support Bot"}

	rel, sig := CorrelateShadowFindings([]models.ShadowFinding{agent, external, bedrock, unrelated}, time.Now())

	assert.Empty(t, rel[agent.ID], "a Foundry agent that is not external is not matched")
	assert.Empty(t, sig[agent.ID])

	require.Len(t, rel[external.ID], 1)
	assert.Equal(t, "possible_duplicate", rel[external.ID][0].Relation)
	assert.Equal(t, bedrock.ID.String(), rel[external.ID][0].FindingID)
	assert.Equal(t, sa.SigFoundryPossibleDupOf, sig[external.ID][0].Code)
	assert.Empty(t, rel[unrelated.ID], "only Foundry external agents are matched by name")
}

// ---- persistence input: retention and the evidence catalog ----

func TestToShadowApply_AppliesRetentionAndCatalog(t *testing.T) {
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	nan := math.NaN()
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out := &sa.Result{
		Findings: []*sa.Finding{
			{
				Kind: models.ShadowKindPlatformAgent, ExternalID: "agent_1", DisplayName: "Bot", State: "active",
				ProviderCreatedAt: &created,
				OwnerHint:         models.ShadowOwnerHint{Email: "dev@example.com", Source: "GET /v1/agents"},
				Attributes: map[string]any{
					"model": "gpt-4.1", "instructions": shadowPromptMark, "system": shadowPromptMark,
					"sessionCount": 4, "models": []string{"x"},
				},
				Signals: []sa.Signal{
					{Code: sa.SigOpenAIAgentObject, Source: "GET /v1/agents", Observed: map[string]any{
						"toolCount": 1, "prompt": shadowPromptMark + strings.Repeat("x", 100), "nan": nan,
					}},
					{Code: "openai.plain_usage", Source: "GET /v1/organization/usage/completions"},
				},
			},
			// No agent evidence (only a removed code): not stored.
			{Kind: models.ShadowKindRuntime, ExternalID: "rt", Signals: []sa.Signal{{Code: "bedrock.runtime_in_use"}}},
			{Kind: "api_key", ExternalID: "key_1", Signals: []sa.Signal{{Code: sa.SigOpenAIAgentObject}}},
			{Kind: "tool_infrastructure", ExternalID: "gw", Signals: []sa.Signal{{Code: sa.SigOpenAIAgentObject}}},
			{Kind: models.ShadowKindPlatformAgent, ExternalID: " ", Signals: []sa.Signal{{Code: sa.SigOpenAIAgentObject}}},
		},
	}
	scan := &models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU}
	conn := &models.ShadowConnection{ID: uuid.New(), Provider: models.ShadowProviderOpenAI}
	started := time.Now().UTC()
	in := toShadowApply(scan, conn, out, started, started, true, log)

	assert.True(t, in.Complete)
	assert.Equal(t, started, in.ScanStartedAt)
	require.Len(t, in.Findings, 1, "only agent kinds with agent evidence and an ID are stored")
	f := in.Findings[0]
	assert.Equal(t, models.ShadowProviderOpenAI, f.Finding.Provider)
	assert.Equal(t, "active", f.Finding.State)
	assert.Equal(t, &created, f.Finding.ProviderCreatedAt)
	assert.Equal(t, "dev@example.com", f.Finding.OwnerHint.Email, "an owner the listing gives is kept")
	assert.Equal(t, map[string]any{"model": "gpt-4.1"}, f.Finding.Attributes, "only allow-listed attributes")
	require.Len(t, f.Signals, 1, "unknown and removed evidence codes are dropped")
	assert.Equal(t, sa.SigOpenAIAgentObject, f.Signals[0].Code)
	assert.Equal(t, "GET /v1/agents", f.Signals[0].Source)
	assert.NotContains(t, f.Signals[0].Observed, "prompt", "long free text is not observed data")
	assert.NotContains(t, f.Signals[0].Observed, "nan")
	assert.EqualValues(t, 1, f.Signals[0].Observed["toolCount"])

	stored, err := json.Marshal(in)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), shadowPromptMark)
	assert.NotContains(t, logs.String(), shadowPromptMark)

	assert.False(t, toShadowApply(scan, conn, out, started, started, false, log).Complete)
}

func TestValidShadowKind_AgentsOnly(t *testing.T) {
	assert.True(t, validShadowKind(models.ShadowKindPlatformAgent))
	assert.True(t, validShadowKind(models.ShadowKindRuntime))
	for _, k := range []string{"api_key", "principal", "tool_infrastructure", "project", "deployment", ""} {
		assert.False(t, validShadowKind(k), k)
	}
}

func TestConnectionOutcome(t *testing.T) {
	ok := models.ShadowScanCallResult{Call: "a", Outcome: models.ShadowCallOK}
	skip := models.ShadowScanCallResult{Call: "b", Outcome: models.ShadowCallSkipped}
	denied := models.ShadowScanCallResult{Call: "c", Outcome: models.ShadowCallPermissionDenied, Message: "no"}
	auth := models.ShadowScanCallResult{Call: "d", Outcome: models.ShadowCallAuthFailed, Message: "bad key"}
	o, _ := connectionOutcome([]models.ShadowScanCallResult{ok, skip})
	assert.Equal(t, shadowConnectionOK, o)
	o, msg := connectionOutcome([]models.ShadowScanCallResult{ok, denied})
	assert.Equal(t, shadowConnectionPartial, o)
	assert.Contains(t, msg, "1 of 2 calls failed")
	o, _ = connectionOutcome([]models.ShadowScanCallResult{auth, skip})
	assert.Equal(t, models.ShadowCallAuthFailed, o)
	o, _ = connectionOutcome([]models.ShadowScanCallResult{denied})
	assert.Equal(t, shadowConnectionFailed, o)
}

// ---- scanner ----

type fakeShadowConnector struct {
	provider string
	scan     func(conn sa.Connection) *sa.Result
}

func (f *fakeShadowConnector) Provider() string { return f.provider }
func (f *fakeShadowConnector) Scan(_ context.Context, conn sa.Connection) *sa.Result {
	return f.scan(conn)
}

func (f *fakeShadowConnector) Test(context.Context, sa.Connection) models.ShadowConnectionTestInfo {
	return models.ShadowConnectionTestInfo{OK: true, Checks: []models.ShadowConnectionCheck{}}
}

type shadowScannerHarness struct {
	mu       sync.Mutex
	applied  map[string]repositories.ShadowConnectionApply
	finished *models.ShadowScan
	repo     *repomocks.ShadowAgentRepositoryMock
}

// newShadowScannerHarness runs one claimed scan over conns with an in-memory
// repository that records what each connection applied.
func newShadowScannerHarness(scan models.ShadowScan, conns ...models.ShadowConnection) *shadowScannerHarness {
	h := &shadowScannerHarness{applied: map[string]repositories.ShadowConnectionApply{}}
	byID := map[uuid.UUID]models.ShadowConnection{}
	for _, c := range conns {
		byID[c.ID] = c
	}
	h.repo = &repomocks.ShadowAgentRepositoryMock{
		ClaimDueScansFunc: func(context.Context, time.Time, time.Duration, int) ([]models.ShadowScan, error) {
			return []models.ShadowScan{scan}, nil
		},
		GetConnectionFunc: func(_ context.Context, _ string, id uuid.UUID) (*models.ShadowConnection, error) {
			c, ok := byID[id]
			if !ok {
				return nil, gorm.ErrRecordNotFound
			}
			return &c, nil
		},
		// Connections run concurrently, so the mock's callbacks are too.
		ApplyConnectionResultFunc: func(_ context.Context, in repositories.ShadowConnectionApply) (int, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.applied[in.ConnectionID.String()] = in
			return len(in.Findings), nil
		},
		ListOrgFindingsFunc: func(context.Context, string) ([]models.ShadowFinding, error) { return []models.ShadowFinding{}, nil },
		ApplyCorrelationFunc: func(context.Context, string, map[uuid.UUID][]models.ShadowFindingRelation,
			map[uuid.UUID][]models.ShadowFindingSignal,
		) error {
			return nil
		},
		FinishScanFunc: func(_ context.Context, s *models.ShadowScan) error {
			h.finished = s
			return nil
		},
		RenewScanLeaseFunc: func(context.Context, uuid.UUID, time.Time) error { return nil },
	}
	return h
}

func agentResult(id string, calls ...models.ShadowScanCallResult) *sa.Result {
	return &sa.Result{
		Findings: []*sa.Finding{{
			Kind: models.ShadowKindPlatformAgent, ExternalID: id,
			Signals: []sa.Signal{{Code: sa.SigOpenAIAgentObject, Source: "GET /v1/agents"}},
		}},
		Calls: calls,
	}
}

// One platform failing (here, panicking) never breaks the others: its
// connection fails, the other's agents are saved, the scan is partial.
func TestShadowScannerIsolatesFailingProviders(t *testing.T) {
	enc, err := (&shadowAgentService{encryptionKey: shadowTestKey}).encryptSecret(models.ShadowConnectionSecret{ScopedKeys: map[string]string{"proj_a": shadowSecretKey}})
	require.NoError(t, err)
	good := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Name: "good", Provider: models.ShadowProviderOpenAI, SecretEncrypted: enc}
	bad := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Name: "bad", Provider: models.ShadowProviderAnthropic, SecretEncrypted: enc}
	connectors := ShadowConnectors{
		models.ShadowProviderOpenAI: &fakeShadowConnector{provider: models.ShadowProviderOpenAI, scan: func(conn sa.Connection) *sa.Result {
			assert.Equal(t, shadowSecretKey, conn.Secret.ScopedKeys["proj_a"], "the connector gets the decrypted secret")
			return agentResult("agent_1", models.ShadowScanCallResult{Call: "GET /v1/agents", Outcome: models.ShadowCallOK, Items: 1})
		}},
		models.ShadowProviderAnthropic: &fakeShadowConnector{provider: models.ShadowProviderAnthropic, scan: func(sa.Connection) *sa.Result {
			panic("connector bug")
		}},
	}
	started := time.Now().UTC()
	scan := models.ShadowScan{
		ID: uuid.New(), OUID: shadowTestOU, Status: models.ShadowScanRunning, Attempts: 1,
		StartedAt: &started, ConnectionIDs: []string{good.ID.String(), bad.ID.String()},
	}
	h := newShadowScannerHarness(scan, good, bad)
	logs := &bytes.Buffer{}
	scanner := NewShadowAgentScanner(h.repo, connectors, shadowTestKey, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	scanner.RunOnce(context.Background())

	require.NotNil(t, h.finished)
	assert.Equal(t, models.ShadowScanPartial, h.finished.Status)
	require.Len(t, h.finished.Results, 2)
	byName := map[string]models.ShadowScanConnectionResult{}
	for _, r := range h.finished.Results {
		byName[r.ConnectionName] = r
	}
	assert.Equal(t, shadowConnectionOK, byName["good"].Outcome)
	assert.Equal(t, 1, byName["good"].FindingsCount)
	assert.Equal(t, shadowConnectionFailed, byName["bad"].Outcome)
	assert.Contains(t, byName["bad"].Calls[0].Message, "internal error")
	assert.True(t, h.applied[good.ID.String()].Complete, "every call succeeded: the listing is complete")
	assert.False(t, h.applied[bad.ID.String()].Complete, "a failed connection never marks an agent as gone")
	assert.Equal(t, started, h.applied[good.ID.String()].ScanStartedAt)
	assert.NotContains(t, logs.String(), "SECRETSECRET", "the credential is never logged")
}

// A listing is complete only when every call succeeded or was skipped on
// purpose; then the repository may mark agents that were not returned.
func TestShadowScannerCompleteListing(t *testing.T) {
	enc, err := (&shadowAgentService{encryptionKey: shadowTestKey}).encryptSecret(models.ShadowConnectionSecret{})
	require.NoError(t, err)
	cases := map[string]struct {
		calls    []models.ShadowScanCallResult
		complete bool
	}{
		"all ok":             {[]models.ShadowScanCallResult{{Call: "a", Outcome: models.ShadowCallOK}}, true},
		"ok and skipped":     {[]models.ShadowScanCallResult{{Call: "a", Outcome: models.ShadowCallOK}, {Call: "b", Outcome: models.ShadowCallSkipped}}, true},
		"one call failed":    {[]models.ShadowScanCallResult{{Call: "a", Outcome: models.ShadowCallOK}, {Call: "b", Outcome: models.ShadowCallPermissionDenied}}, false},
		"page limit reached": {[]models.ShadowScanCallResult{{Call: "a", Outcome: models.ShadowCallOK}, {Call: "a (page limit)", Outcome: models.ShadowCallFailed}}, false},
		"auth failed":        {[]models.ShadowScanCallResult{{Call: "a", Outcome: models.ShadowCallAuthFailed}}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Name: "c", Provider: models.ShadowProviderOpenAI, SecretEncrypted: enc}
			started := time.Now().UTC()
			scan := models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU, Attempts: 1, StartedAt: &started, ConnectionIDs: []string{c.ID.String()}}
			h := newShadowScannerHarness(scan, c)
			connectors := ShadowConnectors{models.ShadowProviderOpenAI: &fakeShadowConnector{
				provider: models.ShadowProviderOpenAI, scan: func(sa.Connection) *sa.Result { return agentResult("agent_1", tc.calls...) },
			}}
			NewShadowAgentScanner(h.repo, connectors, shadowTestKey, discardLogger()).RunOnce(context.Background())
			assert.Equal(t, tc.complete, h.applied[c.ID.String()].Complete)
			assert.Len(t, h.applied[c.ID.String()].Findings, 1, "the agents found are saved either way")
		})
	}
}

func TestShadowScannerGivesUpAfterMaxAttempts(t *testing.T) {
	scan := models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU, Attempts: shadowScanMaxAttempts + 1, ConnectionIDs: []string{uuid.New().String()}}
	var finished *models.ShadowScan
	repo := &repomocks.ShadowAgentRepositoryMock{
		ClaimDueScansFunc: func(context.Context, time.Time, time.Duration, int) ([]models.ShadowScan, error) {
			return []models.ShadowScan{scan}, nil
		},
		FinishScanFunc: func(_ context.Context, s *models.ShadowScan) error {
			finished = s
			return nil
		},
	}
	NewShadowAgentScanner(repo, ShadowConnectors{}, shadowTestKey, discardLogger()).RunOnce(context.Background())
	require.NotNil(t, finished)
	assert.Equal(t, models.ShadowScanFailed, finished.Status)
}

func TestShadowDeleteAgents(t *testing.T) {
	id := uuid.New()
	repo := &repomocks.ShadowAgentRepositoryMock{
		DeleteFindingFunc: func(_ context.Context, ou string, got uuid.UUID) (bool, error) {
			return ou == shadowTestOU && got == id, nil
		},
		DeleteFindingsFunc: func(context.Context, string) (int64, error) { return 4, nil },
	}
	svc := newShadowSvc(repo)
	require.NoError(t, svc.DeleteFinding(context.Background(), shadowTestOU, id))
	assert.ErrorIs(t, svc.DeleteFinding(context.Background(), shadowTestOU, uuid.New()), utils.ErrShadowFindingNotFound)
	assert.ErrorIs(t, svc.DeleteFinding(context.Background(), "other-ou", id), utils.ErrShadowFindingNotFound)
	n, err := svc.DeleteFindings(context.Background(), shadowTestOU)
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	repo.DeleteFindingFunc = func(context.Context, string, uuid.UUID) (bool, error) { return false, errors.New("db down") }
	err = svc.DeleteFinding(context.Background(), shadowTestOU, id)
	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrShadowFindingNotFound)
}
