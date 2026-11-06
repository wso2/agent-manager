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

package openchoreo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// pushSecretAfterCreateConflict runs PushSecret against a secret API that has no
// secret by this name, rejects the create as a conflict, and fails the fallback
// update with updateErr.
func pushSecretAfterCreateConflict(t *testing.T, updateErr error) error {
	t.Helper()
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretFunc: func(context.Context, string, string) (*occlient.SecretInfo, error) {
			return nil, utils.ErrNotFound
		},
		CreateSecretFunc: func(context.Context, string, occlient.CreateSecretRequest) (*occlient.SecretInfo, error) {
			return nil, utils.ErrConflict
		},
		UpdateSecretFunc: func(context.Context, string, string, occlient.UpdateSecretRequest) (*occlient.SecretInfo, error) {
			return nil, updateErr
		},
	}
	c := &Client{oc: oc}
	_, err := c.PushSecret(
		context.Background(),
		secretmanagersvc.SecretLocation{OrgName: "acme", EntityName: "my-agent-agent-identity-dev"},
		map[string]string{"client-secret": "value"},
		&secretmanagersvc.SecretMetadata{ManagedBy: "amp-agent-manager"},
	)
	return err
}

// A SecretReference created outside the secret API takes the name, so the create
// conflicts, yet the API has no secret to update. That case, and only that case,
// is reported as ErrReferenceNameTaken, which callers rely on to recover.
func TestPushSecret_NameTakenByUnmanagedReference_ReturnsSentinel(t *testing.T) {
	err := pushSecretAfterCreateConflict(t, utils.ErrNotFound)

	require.Error(t, err)
	assert.ErrorIs(t, err, secretmanagersvc.ErrReferenceNameTaken)
	assert.ErrorIs(t, err, utils.ErrNotFound, "the underlying not-found must stay reachable")
}

func TestPushSecret_OtherUpdateFailureAfterConflict_IsNotNameTaken(t *testing.T) {
	boom := errors.New("boom")

	err := pushSecretAfterCreateConflict(t, boom)

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, secretmanagersvc.ErrReferenceNameTaken,
		"only a not-found update means the name is held by a reference this client cannot manage")
}

// deleteByNameClient returns a Client over a secret API holding one secret,
// named existingName with the given managed-by label, that records deletes.
func deleteByNameClient(existingName, managedBy string, deleted *[]string) *Client {
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretFunc: func(_ context.Context, _, name string) (*occlient.SecretInfo, error) {
			if name != existingName {
				return nil, utils.ErrNotFound
			}
			return &occlient.SecretInfo{Name: name, Labels: map[string]string{labelKeyManagedBy: managedBy}}, nil
		},
		DeleteSecretFunc: func(_ context.Context, _, name string) error {
			*deleted = append(*deleted, name)
			return nil
		},
	}
	return &Client{oc: oc}
}

// DeleteSecretByName targets the given name, not one derived from a location,
// so a secret created under a name that a location no longer produces (for
// example after a configuration rename) is still deleted.
func TestDeleteSecretByName_DeletesTheNamedSecret(t *testing.T) {
	var deleted []string
	c := deleteByNameClient("old-cfg-dev-proxy-secrets", "amp-agent-manager", &deleted)

	err := c.DeleteSecretByName(context.Background(), "acme", "old-cfg-dev-proxy-secrets",
		&secretmanagersvc.SecretMetadata{ManagedBy: "amp-agent-manager"})

	require.NoError(t, err)
	assert.Equal(t, []string{"old-cfg-dev-proxy-secrets"}, deleted)
}

// DeleteSecretByName keeps DeleteSecret's guarantees: it is idempotent and
// never deletes a secret another owner manages.
func TestDeleteSecretByName_SkipsMissingAndUnmanagedSecrets(t *testing.T) {
	metadata := &secretmanagersvc.SecretMetadata{ManagedBy: "amp-agent-manager"}

	t.Run("missing", func(t *testing.T) {
		var deleted []string
		c := deleteByNameClient("other", "amp-agent-manager", &deleted)

		require.NoError(t, c.DeleteSecretByName(context.Background(), "acme", "gone-secrets", metadata))
		assert.Empty(t, deleted)
	})

	t.Run("managed by another owner", func(t *testing.T) {
		var deleted []string
		c := deleteByNameClient("shared-secrets", "someone-else", &deleted)

		require.NoError(t, c.DeleteSecretByName(context.Background(), "acme", "shared-secrets", metadata))
		assert.Empty(t, deleted)
	})
}
