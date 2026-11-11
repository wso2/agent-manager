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

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

var testEncryptionKey = []byte("01234567890123456789012345678901") // 32 bytes for AES-256

// A new org gets both a publisher and a scheduler credential, each stored under its own secret reference.
func TestEnsureCredentials_NewOrg_StoresPublisherAndSchedulerCredentials(t *testing.T) {
	thunderMock := &clientmocks.ThunderClientMock{
		EnsurePublisherAppFunc: func(_ context.Context, orgName, _ string) (string, string, bool, error) {
			return "amp-publisher-" + orgName, "publisher-secret", true, nil
		},
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			return appName, "scheduler-secret", true, nil
		},
	}

	var publisherUpserts []*models.OrgPublisherCredential
	credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgPublisherCredential, error) {
			return nil, gorm.ErrRecordNotFound
		},
		UpsertFunc: func(cred *models.OrgPublisherCredential) error {
			publisherUpserts = append(publisherUpserts, cred)
			return nil
		},
	}

	var schedulerUpserts []*models.OrgSchedulerCredential
	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgSchedulerCredential, error) {
			return nil, gorm.ErrRecordNotFound
		},
		UpsertFunc: func(cred *models.OrgSchedulerCredential) error {
			schedulerUpserts = append(schedulerUpserts, cred)
			return nil
		},
	}

	secretClient := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref-" + location.EntityName, nil
		},
	}

	p := &publisherCredentialProvisioner{
		thunderClient:     thunderMock,
		secretClient:      secretClient,
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
	}

	// The SecretReference names returned by CreateSecret are stored as-is.
	creds, err := p.EnsureCredentials(context.Background(), "acme", "org-uuid-1")
	require.NoError(t, err)
	assert.Equal(t, "amp-publisher-acme", creds.ClientID)
	assert.Equal(t, "ref-amp-publisher-acme", creds.SecretRefName)

	require.Len(t, publisherUpserts, 1)
	assert.Equal(t, "amp-publisher-acme", publisherUpserts[0].ClientID)
	assert.Equal(t, "ref-amp-publisher-acme", publisherUpserts[0].SecretKVPath)

	require.Len(t, schedulerUpserts, 1)
	assert.Equal(t, "amp-scheduler-acme", schedulerUpserts[0].ClientID)
	assert.Equal(t, "ref-amp-scheduler-acme", schedulerUpserts[0].SecretKVPath)
}

// Idempotent path: an org with stored credentials gets them back without provisioning new ones.
func TestEnsureCredentials_ExistingOrg_ReturnsStoredCredentials(t *testing.T) {
	thunderMock := &clientmocks.ThunderClientMock{}
	credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgPublisherCredential, error) {
			return &models.OrgPublisherCredential{
				ClientID: "amp-publisher-acme", SecretKVPath: "kv/publisher", SecretKey: "client-secret",
			}, nil
		},
	}
	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgSchedulerCredential, error) {
			return &models.OrgSchedulerCredential{
				ClientID: "amp-scheduler-acme", SecretKVPath: "kv/scheduler", SecretKey: "client-secret",
			}, nil
		},
	}

	p := &publisherCredentialProvisioner{
		thunderClient:     thunderMock,
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
	}

	creds, err := p.EnsureCredentials(context.Background(), "acme", "org-uuid-1")
	require.NoError(t, err)
	assert.Equal(t, "amp-publisher-acme", creds.ClientID)
}
