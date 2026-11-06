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

// UNIT test guarding how a run's publisher credentials are chosen.
//
// The names this produces are consumed as a non-optional secretKeyRef by the
// evaluation job pod, so naming a credential that does not exist does not fail the
// run — it yields an ExternalSecret that can never sync and a pod that waits on it
// forever, and nothing later in the scheduler reclaims either. Substituting a
// plausible default for a missing record is therefore worse than refusing to build
// the run at all, which is what these tests pin down.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestMonitorExecutor_buildPublishingParams(t *testing.T) {
	monitor := &models.Monitor{ID: uuid.New(), OUID: "acme", Name: "my-monitor"}
	runID := uuid.New()

	newExecutor := func(
		credRepo *repomocks.OrgPublisherCredentialRepositoryMock,
		prov PublisherCredentialProvisioner,
	) *monitorExecutor {
		return &monitorExecutor{logger: discardLogger(), credRepo: credRepo, provisioner: prov}
	}

	t.Run("passes the stored SecretReference name without calling the provisioner", func(t *testing.T) {
		credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
			GetByOrgNameFunc: func(ouID string) (*models.OrgPublisherCredential, error) {
				assert.Equal(t, "acme", ouID)
				return &models.OrgPublisherCredential{
					OUID:         "acme",
					ClientID:     "acme-publisher",
					SecretKVPath: "amp-publisher-acme-secrets",
					SecretKey:    "client-secret",
				}, nil
			},
		}
		// Provisioner left unconfigured: provisioning must not run on the hot path.
		e := newExecutor(credRepo, &fakeProvisioner{})

		params, err := e.buildPublishingParams(context.Background(), monitor, runID)

		require.NoError(t, err)
		assert.Equal(t, "acme-publisher", params["clientId"])
		assert.Equal(t, "amp-publisher-acme-secrets", params["secretRefName"])
		assert.NotContains(t, params, "secretKVPath", "the workflow resolves the SecretReference itself")
		assert.NotContains(t, params, "secretKey")
	})

	t.Run("maps a stored legacy KV path to the deterministic SecretReference name", func(t *testing.T) {
		credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgPublisherCredential, error) {
				return &models.OrgPublisherCredential{
					OUID:         "acme",
					ClientID:     "acme-publisher",
					SecretKVPath: "secret/acme/generic/amp-publisher-acme-secrets",
					SecretKey:    "client-secret",
				}, nil
			},
		}
		e := newExecutor(credRepo, &fakeProvisioner{})

		params, err := e.buildPublishingParams(context.Background(), monitor, runID)

		require.NoError(t, err)
		assert.Equal(t, publisherSecretLocation("acme").SecretRefName(), params["secretRefName"])
		assert.NotContains(t, params, "secretKVPath")
	})

	t.Run("provisions on demand when no credentials are stored", func(t *testing.T) {
		credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgPublisherCredential, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		var gotOUID, gotOrgUUID string
		prov := &fakeProvisioner{
			EnsureCredentialsFunc: func(_ context.Context, ouID, orgUUID string) (*PublisherCredentials, error) {
				gotOUID, gotOrgUUID = ouID, orgUUID
				return &PublisherCredentials{
					ClientID:      "provisioned-client",
					SecretRefName: "amp-publisher-acme-secrets",
				}, nil
			},
		}
		e := newExecutor(credRepo, prov)

		params, err := e.buildPublishingParams(context.Background(), monitor, runID)

		require.NoError(t, err)
		assert.Equal(t, "provisioned-client", params["clientId"])
		assert.Equal(t, "amp-publisher-acme-secrets", params["secretRefName"])
		assert.Equal(t, "acme", gotOUID)
		// ouID is the Thunder OU ID, so it is also the orgUUID.
		assert.Equal(t, "acme", gotOrgUUID)
	})

	t.Run("fails the run when credentials cannot be provisioned", func(t *testing.T) {
		boom := errors.New("thunder unreachable")
		credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgPublisherCredential, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		prov := &fakeProvisioner{
			EnsureCredentialsFunc: func(context.Context, string, string) (*PublisherCredentials, error) {
				return nil, boom
			},
		}
		e := newExecutor(credRepo, prov)

		_, err := e.buildPublishingParams(context.Background(), monitor, runID)

		// Building the run anyway would create a workflow that can never start and
		// an ExternalSecret that can never sync, neither of which is ever collected.
		require.ErrorIs(t, err, boom)
	})

	t.Run("falls back to the fixed on-prem app when Thunder is not configured", func(t *testing.T) {
		credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgPublisherCredential, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		e := newExecutor(credRepo, NewStaticPublisherCredentialProvisioner())

		params, err := e.buildPublishingParams(context.Background(), monitor, runID)

		require.NoError(t, err)
		assert.Equal(t, "amp-publisher-client", params["clientId"])
		assert.Equal(t, "amp-publisher-client-secret", params["secretKVPath"])
		assert.Equal(t, "value", params["secretKey"])
		assert.NotContains(t, params, "secretRefName", "static credentials have no SecretReference")
	})

	t.Run("propagates a real lookup error instead of provisioning", func(t *testing.T) {
		boom := errors.New("connection refused")
		credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgPublisherCredential, error) { return nil, boom },
		}
		// Provisioner left unconfigured: a transient DB fault must not mint new apps.
		e := newExecutor(credRepo, &fakeProvisioner{})

		_, err := e.buildPublishingParams(context.Background(), monitor, runID)

		assert.ErrorIs(t, err, boom)
	})
}
