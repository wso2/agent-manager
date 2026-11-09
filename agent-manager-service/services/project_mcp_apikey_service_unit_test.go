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
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// keyCtx is a request context with an audit recorder, which issuing, rotating and revoking a key require.
func keyCtx() context.Context {
	return audit.WithRecorder(context.Background(), audit.NewNoopRecorder())
}

// keyFixture is a bound project proxy with an in-memory key store and the organization's gateways.
type keyFixture struct {
	*bindingFixture
	keys     map[string]models.StoredAPIKey // artifact/name -> key
	gateways []*models.Gateway
	events   *stubEventHub
}

func newKeyFixture(t *testing.T, req models.MCPProxyEnvironmentRequest) *keyFixture {
	t.Helper()
	f := &keyFixture{
		bindingFixture: newBindingFixture(t),
		keys:           map[string]models.StoredAPIKey{},
		gateways:       []*models.Gateway{{UUID: uuid.New()}, {UUID: uuid.New()}},
		events:         &stubEventHub{},
	}
	_, _, err := f.put(t, req)
	require.NoError(t, err)

	keyOf := func(artifact, name string) string { return artifact + "/" + name }
	f.svc.base.apiKeyBroadcaster = apiKeyBroadcaster{
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			GetByOrganizationIDFunc: func(context.Context, string) ([]*models.Gateway, error) { return f.gateways, nil },
		},
		gatewayService: NewGatewayEventsService(f.events),
		apiKeyRepo: &repomocks.APIKeyRepositoryMock{
			// As the repository does, a conflicting upsert updates the value, status and times only.
			UpsertFunc: func(key *models.StoredAPIKey) error {
				k := keyOf(key.ArtifactUUID.String(), key.Name)
				if prev, ok := f.keys[k]; ok {
					prev.APIKeyHash, prev.MaskedAPIKey, prev.Status = key.APIKeyHash, key.MaskedAPIKey, key.Status
					prev.UpdatedAt, prev.ExpiresAt = key.UpdatedAt, key.ExpiresAt
					f.keys[k] = prev
					return nil
				}
				f.keys[k] = *key
				return nil
			},
			DeleteFunc: func(artifact, name string) error {
				delete(f.keys, keyOf(artifact, name))
				return nil
			},
			GetByArtifactAndNameFunc: func(artifact, name string) (*models.StoredAPIKey, error) {
				if key, ok := f.keys[keyOf(artifact, name)]; ok {
					return &key, nil
				}
				return nil, gorm.ErrRecordNotFound
			},
			ListByArtifactFunc: func(_ context.Context, artifact string) ([]models.StoredAPIKey, error) {
				var out []models.StoredAPIKey
				for _, key := range f.keys {
					if key.ArtifactUUID.String() == artifact {
						out = append(out, key)
					}
				}
				return out, nil
			},
		},
	}
	return f
}

func (f *keyFixture) artifact() string {
	_, ee := f.binding()
	return ee.ArtifactUUID.String()
}

func (f *keyFixture) create(t *testing.T, name string) (*models.CreateAPIKeyResponse, error) {
	t.Helper()
	return f.svc.CreateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv,
		&models.CreateAPIKeyRequest{Name: name})
}

// eventAPIIDs is the apiId of each key event the gateways were sent, in order.
func (f *keyFixture) eventAPIIDs(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, e := range f.events.published {
		var envelope struct {
			Payload struct {
				APIID string `json:"apiId"`
			} `json:"payload"`
		}
		require.NoError(t, json.Unmarshal([]byte(e.EventData), &envelope))
		ids = append(ids, envelope.Payload.APIID)
	}
	return ids
}

// A key is issued against the binding's gateway artifact, stored under it, and sent to every gateway of the org.
func TestBindingAPIKey_CreateStoresUnderTheBindingAndSendsToEveryGateway(t *testing.T) {
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})

	resp, err := f.create(t, "ip-users")
	require.NoError(t, err)
	assert.Equal(t, "ip-users", resp.KeyID)
	assert.NotEmpty(t, resp.APIKey, "the value is returned once")

	stored, ok := f.keys[f.artifact()+"/ip-users"]
	require.True(t, ok, "stored under the binding's artifact")
	assert.Equal(t, models.APIKeyPurposeUserManaged, stored.Purpose)
	assert.NotContains(t, stored.APIKeyHash, resp.APIKey, "only the hash is stored")

	require.Len(t, f.events.published, 2, "one event per gateway of the org")
	assert.Equal(t, []string{f.artifact(), f.artifact()}, f.eventAPIIDs(t), "checked against the binding's artifact")

	list, err := f.svc.ListEnvironmentAPIKeys(keyCtx(), testOrg, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	require.Len(t, list.Keys, 1)
	assert.Equal(t, "ip-users", list.Keys[0].Name)
}

// With no gateway connected yet the key is still stored; a gateway receives it in its key sync.
func TestBindingAPIKey_CreateWithNoGatewayIsStored(t *testing.T) {
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
	f.gateways = nil

	_, err := f.create(t, "ip-users")
	require.NoError(t, err)
	assert.Contains(t, f.keys, f.artifact()+"/ip-users")
	assert.Empty(t, f.events.published)
}

func TestBindingAPIKey_CreateRefusals(t *testing.T) {
	t.Run("security off is 409 and nothing is stored", func(t *testing.T) {
		off := false
		f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{Enabled: &off}})
		_, err := f.create(t, "ip-users")
		require.ErrorIs(t, err, utils.ErrMCPAPIKeySecurityDisabled)
		assert.Empty(t, f.keys)
		assert.Empty(t, f.events.published)
	})

	t.Run("a name the binding already has is 409 and the key keeps its value", func(t *testing.T) {
		f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
		_, err := f.create(t, "ip-users")
		require.NoError(t, err)
		before := f.keys[f.artifact()+"/ip-users"].APIKeyHash

		_, err = f.create(t, "ip-users")
		require.ErrorIs(t, err, utils.ErrMCPAPIKeyExists)
		assert.Equal(t, before, f.keys[f.artifact()+"/ip-users"].APIKeyHash)
	})

	t.Run("an environment the proxy is not bound in is 404", func(t *testing.T) {
		f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
		_, err := f.svc.CreateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, "production",
			&models.CreateAPIKeyRequest{Name: "ip-users"})
		require.ErrorIs(t, err, utils.ErrMCPProxyEnvironmentNotBound)
	})

	t.Run("a proxy of another project is 404", func(t *testing.T) {
		f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
		_, err := f.svc.CreateEnvironmentAPIKey(keyCtx(), testOrg, "p2", uuid.NewString(), bindingHandle, bindingEnv,
			&models.CreateAPIKeyRequest{Name: "ip-users"})
		require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	})

	t.Run("an expiresAt that is not an RFC 3339 time is 400", func(t *testing.T) {
		f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
		bad := "next tuesday"
		_, err := f.svc.CreateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv,
			&models.CreateAPIKeyRequest{Name: "ip-users", ExpiresAt: &bad})
		require.ErrorIs(t, err, utils.ErrBadRequest)
		assert.Empty(t, f.keys)
	})

	t.Run("an expiresAt that is not in the future is 400", func(t *testing.T) {
		for name, at := range map[string]string{
			"past": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			"now":  time.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
		} {
			t.Run(name, func(t *testing.T) {
				f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
				_, err := f.svc.CreateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle,
					bindingEnv, &models.CreateAPIKeyRequest{Name: "ip-users", ExpiresAt: &at})
				require.ErrorIs(t, err, utils.ErrBadRequest)
				assert.Empty(t, f.keys)
				assert.Empty(t, f.events.published, "nothing is sent to the gateways")
			})
		}
	})

	t.Run("a name shorter than three characters is 400", func(t *testing.T) {
		f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
		_, err := f.create(t, "ab")
		require.ErrorIs(t, err, utils.ErrBadRequest)
		assert.Empty(t, f.keys)
	})
}

func TestBindingAPIKey_RotateGivesANewValue(t *testing.T) {
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
	created, err := f.create(t, "ip-users")
	require.NoError(t, err)
	before := f.keys[f.artifact()+"/ip-users"].APIKeyHash

	rotated, err := f.svc.RotateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv,
		"ip-users", nil)
	require.NoError(t, err)
	assert.NotEqual(t, created.APIKey, rotated.APIKey)
	assert.NotEqual(t, before, f.keys[f.artifact()+"/ip-users"].APIKeyHash)
	assert.Len(t, f.events.published, 4, "the new value goes to every gateway")
	assert.Equal(t, f.artifact(), f.eventAPIIDs(t)[3])

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	_, err = f.svc.RotateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv,
		"ip-users", &models.RotateAPIKeyRequest{ExpiresAt: &past})
	require.ErrorIs(t, err, utils.ErrBadRequest, "a past expiresAt is refused on rotate too")
	assert.Len(t, f.events.published, 4, "a refused rotation sends nothing")

	_, err = f.svc.RotateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv,
		"unknown", nil)
	require.ErrorIs(t, err, utils.ErrMCPAPIKeyNotFound)
	assert.NotContains(t, f.keys, f.artifact()+"/unknown", "rotating an unknown key creates nothing")
}

// A key issued before security was turned off can still be revoked.
func TestBindingAPIKey_RevokeWorksWhateverTheSecurity(t *testing.T) {
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
	_, err := f.create(t, "ip-users")
	require.NoError(t, err)

	off := false
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{Enabled: &off}})
	require.NoError(t, err)

	require.NoError(t, f.svc.RevokeEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv, "ip-users"))
	assert.Empty(t, f.keys)
	assert.Len(t, f.events.published, 4, "revoked on every gateway")

	err = f.svc.RevokeEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv, "ip-users")
	require.ErrorIs(t, err, utils.ErrMCPAPIKeyNotFound)
}

func (f *keyFixture) issueTest(t *testing.T, forUser string, expiresAt *string) (*models.IssueTestAPIKeyResponse, error) {
	t.Helper()
	return f.svc.IssueEnvironmentTestAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv,
		&models.IssueBindingTestAPIKeyRequest{ForUser: forUser, ExpiresAt: expiresAt})
}

// age sets the stored key's issue time, so the slot order is not left to the clock.
func (f *keyFixture) age(name string, at time.Time) {
	k := f.artifact() + "/" + name
	key := f.keys[k]
	key.UpdatedAt = at
	f.keys[k] = key
}

// A test key is stored under the binding as a test key, expires in an hour by default, is sent to every gateway and
// is not listed.
func TestBindingTestAPIKey_IssuedUnlistedWithAnHourDefault(t *testing.T) {
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})

	before := time.Now().UTC()
	resp, err := f.issueTest(t, "user-1", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.APIKey)
	assert.True(t, strings.HasPrefix(resp.KeyID, models.APIKeyTestKeyPrefix), resp.KeyID)
	assert.True(t, strings.HasSuffix(resp.KeyID, "-1"), "first slot")
	assert.NotContains(t, resp.KeyID, "user-1", "the user is only hashed into the name")

	expires, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	require.NoError(t, err)
	assert.WithinDuration(t, before.Add(time.Hour), expires, 5*time.Second)

	stored := f.keys[f.artifact()+"/"+resp.KeyID]
	assert.Equal(t, models.APIKeyPurposeTest, stored.Purpose)
	require.NotNil(t, stored.ExpiresAt)
	assert.Equal(t, []string{f.artifact(), f.artifact()}, f.eventAPIIDs(t), "sent to every gateway")

	list, err := f.svc.ListEnvironmentAPIKeys(keyCtx(), testOrg, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	assert.Empty(t, list.Keys, "test keys are not listed")
}

// A user fills three slots; the fourth key replaces the slot issued longest ago. Another user has their own slots.
func TestBindingTestAPIKey_ThreeSlotsPerUserOldestReplaced(t *testing.T) {
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})

	base := time.Now().UTC().Add(-time.Hour)
	var names []string
	values := map[string]string{}
	for i := range 3 {
		resp, err := f.issueTest(t, "user-1", nil)
		require.NoError(t, err)
		names = append(names, resp.KeyID)
		values[resp.KeyID] = f.keys[f.artifact()+"/"+resp.KeyID].APIKeyHash
		f.age(resp.KeyID, base.Add(time.Duration(i)*time.Minute))
	}
	assert.Len(t, f.keys, 3)
	// Make the second slot the oldest.
	f.age(names[1], base.Add(-time.Minute))

	resp, err := f.issueTest(t, "user-1", nil)
	require.NoError(t, err)
	assert.Equal(t, names[1], resp.KeyID, "the slot issued longest ago is reused")
	assert.Len(t, f.keys, 3, "no fourth key")
	replaced := f.keys[f.artifact()+"/"+names[1]]
	assert.NotEqual(t, values[names[1]], replaced.APIKeyHash, "its old value stops working")
	assert.Equal(t, models.APIKeyPurposeTest, replaced.Purpose)

	other, err := f.issueTest(t, "user-2", nil)
	require.NoError(t, err)
	assert.NotContains(t, names, other.KeyID, "another user has their own slots")
	assert.Len(t, f.keys, 4)
}

// The user key routes neither reach a test key nor take a test key name.
func TestBindingTestAPIKey_UserRoutesDoNotReachTestKeys(t *testing.T) {
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
	resp, err := f.issueTest(t, "user-1", nil)
	require.NoError(t, err)

	_, err = f.svc.RotateEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv,
		resp.KeyID, &models.RotateAPIKeyRequest{})
	require.ErrorIs(t, err, utils.ErrMCPAPIKeyNotFound)
	err = f.svc.RevokeEnvironmentAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv, resp.KeyID)
	require.ErrorIs(t, err, utils.ErrMCPAPIKeyNotFound)
	_, err = f.create(t, models.APIKeyTestKeyPrefix+"mine")
	require.ErrorIs(t, err, utils.ErrBadRequest)

	assert.Contains(t, f.keys, f.artifact()+"/"+resp.KeyID, "the test key is untouched")
}

func TestBindingTestAPIKey_Refusals(t *testing.T) {
	ptr := func(s string) *string { return &s }
	now := time.Now().UTC()

	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{})
	for name, tc := range map[string]struct {
		forUser   string
		expiresAt *string
	}{
		"no user":        {forUser: "  "},
		"user too long":  {forUser: strings.Repeat("u", 257)},
		"bad expiresAt":  {forUser: "u", expiresAt: ptr("tomorrow")},
		"past expiresAt": {forUser: "u", expiresAt: ptr(now.Add(-time.Minute).Format(time.RFC3339))},
		"over 24 hours":  {forUser: "u", expiresAt: ptr(now.Add(25 * time.Hour).Format(time.RFC3339))},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.issueTest(t, tc.forUser, tc.expiresAt)
			require.ErrorIs(t, err, utils.ErrBadRequest)
		})
	}
	assert.Empty(t, f.keys, "nothing stored")

	expiresAt := now.Add(2 * time.Hour).Truncate(time.Second)
	resp, err := f.issueTest(t, "u", ptr(expiresAt.Format(time.RFC3339)))
	require.NoError(t, err)
	assert.Equal(t, expiresAt.Format(time.RFC3339), resp.ExpiresAt, "a given expiresAt is kept")
}

func TestBindingTestAPIKey_SecurityOffOrNoBinding(t *testing.T) {
	off := false
	f := newKeyFixture(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{Enabled: &off}})
	_, err := f.issueTest(t, "u", nil)
	require.ErrorIs(t, err, utils.ErrMCPAPIKeySecurityDisabled)

	_, err = f.svc.IssueEnvironmentTestAPIKey(keyCtx(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, "production",
		&models.IssueBindingTestAPIKeyRequest{ForUser: "u"})
	require.ErrorIs(t, err, utils.ErrMCPProxyEnvironmentNotBound)
	assert.Empty(t, f.keys)
}
