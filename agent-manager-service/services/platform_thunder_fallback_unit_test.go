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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func withPlatformThunderConfig(t *testing.T, baseURL, clientID, clientSecret string) {
	t.Helper()
	orig := config.GetConfig().Thunder
	t.Cleanup(func() { config.GetConfig().Thunder = orig })
	config.GetConfig().Thunder.BaseURL = baseURL
	config.GetConfig().Thunder.ClientID = clientID
	config.GetConfig().Thunder.ClientSecret = clientSecret
}

func unregisteredThunderRepos() (*repomocks.EnvThunderSystemClientRepositoryMock, *repomocks.EnvThunderURLRepositoryMock) {
	return &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}, &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
}

func TestEnvThunderReaders_FallBackToPlatformThunderForDefaultEnvironment(t *testing.T) {
	withPlatformThunderConfig(t, "http://thunder.example.test", "amp-system-client", "s3cret")
	secrets, urls := unregisteredThunderRepos()

	clientID, clientSecret, err := NewEnvThunderSecretReader(secrets, nil)(context.Background(), "ou-1", models.DefaultEnvironmentName)
	require.NoError(t, err)
	assert.Equal(t, "amp-system-client", clientID)
	assert.Equal(t, "s3cret", clientSecret)

	url, callerSupplied, err := NewEnvThunderURLReader(urls)(context.Background(), "ou-1", models.DefaultEnvironmentName)
	require.NoError(t, err)
	assert.Equal(t, "http://thunder.example.test", url)
	assert.True(t, callerSupplied, "the platform URL is a full origin, not a handle")
}

func TestEnvThunderReaders_NoFallbackWithoutPlatformThunder(t *testing.T) {
	withPlatformThunderConfig(t, "", "", "")
	secrets, urls := unregisteredThunderRepos()

	_, _, err := NewEnvThunderSecretReader(secrets, nil)(context.Background(), "ou-1", models.DefaultEnvironmentName)
	assert.ErrorIs(t, err, thundersvc.ErrThunderNotProvisioned)

	url, _, err := NewEnvThunderURLReader(urls)(context.Background(), "ou-1", models.DefaultEnvironmentName)
	require.NoError(t, err)
	assert.Empty(t, url)
}

func TestEnvThunderReaders_NoFallbackForOtherEnvironments(t *testing.T) {
	withPlatformThunderConfig(t, "http://thunder.example.test", "amp-system-client", "s3cret")
	secrets, urls := unregisteredThunderRepos()

	_, _, err := NewEnvThunderSecretReader(secrets, nil)(context.Background(), "ou-1", "prod")
	assert.ErrorIs(t, err, thundersvc.ErrThunderNotProvisioned)

	url, _, err := NewEnvThunderURLReader(urls)(context.Background(), "ou-1", "prod")
	require.NoError(t, err)
	assert.Empty(t, url)
}

func TestEnvThunderReaders_RegistrationWinsOverPlatformThunder(t *testing.T) {
	withPlatformThunderConfig(t, "http://thunder.example.test", "amp-system-client", "s3cret")
	key := make([]byte, 32)
	encrypted, err := utils.EncryptBytes([]byte("registered-secret"), key)
	require.NoError(t, err)
	secrets := &repomocks.EnvThunderSystemClientRepositoryMock{
		GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
			return &models.EnvThunderSystemClient{ClientID: "registered", ClientSecretEncrypted: encrypted}, nil
		},
	}
	urls := &repomocks.EnvThunderURLRepositoryMock{
		GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
			return &models.EnvThunderURL{ThunderURL: "https://registered.example.test"}, nil
		},
	}

	clientID, clientSecret, err := NewEnvThunderSecretReader(secrets, key)(context.Background(), "ou-1", models.DefaultEnvironmentName)
	require.NoError(t, err)
	assert.Equal(t, "registered", clientID)
	assert.Equal(t, "registered-secret", clientSecret)

	url, _, err := NewEnvThunderURLReader(urls)(context.Background(), "ou-1", models.DefaultEnvironmentName)
	require.NoError(t, err)
	assert.Equal(t, "https://registered.example.test", url)
}
