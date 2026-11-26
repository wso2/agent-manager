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
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// gatewayWithDelivery is a gateway repository holding one gateway, gw-1, with the given secret delivery.
func gatewayWithDelivery(delivery string) *gwrepomocks.GatewayRepositoryMock {
	return &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) {
			return &gwmodels.Gateway{Properties: map[string]interface{}{gwmodels.SecretDeliveryProperty: delivery}}, nil
		},
	}
}

// secretFixture is a gateway, gw-1 in ou-1, with one deployed A2A agent whose upstream auth is the secret at ref.
type secretFixture struct {
	svc     *GatewayInternalAPIService
	secrets *gwtest.Secrets
	ref     string
}

func newSecretFixture(t *testing.T, delivery string) secretFixture {
	t.Helper()
	secrets := gwtest.NewSecrets()
	ref := "a2a-proxies/1/upstream-auth"
	require.NoError(t, secrets.Put(context.Background(), "ou-1", ref, "s3cret"))
	in := proxiedA2AInput()
	in.UpstreamAuth.SecretRef = &ref
	stored, err := generateA2AAgentDeploymentYAML(in)
	require.NoError(t, err)

	deploymentID := uuid.New()
	artifact := gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.MustParse(testArtifactID), Kind: gwmodels.KindAgent}
	repo := &gwrepomocks.DeploymentRepositoryMock{
		GetPullableFunc: func(context.Context, string, uuid.UUID, string) (*gwmodels.Deployment, error) {
			return &gwmodels.Deployment{Content: []byte(stored)}, nil
		},
		ListForGatewaySyncFunc: func(_ context.Context, ouID, gatewayID string) ([]gwmodels.GatewaySyncRow, error) {
			if ouID != "ou-1" || gatewayID != "gw-1" {
				return nil, nil
			}
			return []gwmodels.GatewaySyncRow{
				{ArtifactRef: artifact, DeploymentID: deploymentID.String(), State: gwmodels.GatewaySyncStateDeployed},
				// An undeployed artifact's secrets are not offered.
				{ArtifactRef: artifact, DeploymentID: uuid.NewString(), State: gwmodels.GatewaySyncStateUndeployed},
			}, nil
		},
		ListContentForGatewayFunc: func(_ context.Context, _, _ string, ids []uuid.UUID) ([]gwmodels.GatewayDeploymentContent, error) {
			assert.Equal(t, []uuid.UUID{deploymentID}, ids)
			return []gwmodels.GatewayDeploymentContent{
				{ArtifactRef: artifact, DeploymentID: deploymentID, Content: []byte(stored)},
			}, nil
		},
	}
	svc := NewGatewayInternalAPIService(nil, nil, repo, nil, gatewayWithDelivery(delivery), nil, secrets, gatewayKinds())
	return secretFixture{svc: svc, secrets: secrets, ref: ref}
}

func TestReferenceDeliverySendsAPlaceholderTheGatewayCanFetch(t *testing.T) {
	f := newSecretFixture(t, gwmodels.SecretDeliveryReference)
	ctx := context.Background()

	got, err := f.svc.GetActiveAgentDeploymentByGateway(ctx, testArtifactID, "ou-1", "gw-1")
	require.NoError(t, err)
	assert.NotContains(t, got[testArtifactID], "s3cret")

	listed, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", true)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Contains(t, got[testArtifactID], secretPlaceholder(listed[0].Handle))
	assert.Equal(t, "ACTIVE", listed[0].Status)
	require.NotNil(t, listed[0].Value)
	assert.Equal(t, "s3cret", *listed[0].Value)

	value, err := f.svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", listed[0].Handle)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", value)
}

func TestGatewaySecretsListsNoValuesUnlessAsked(t *testing.T) {
	f := newSecretFixture(t, gwmodels.SecretDeliveryReference)
	listed, err := f.svc.ListGatewaySecrets(context.Background(), "ou-1", "gw-1", false)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Nil(t, listed[0].Value)
	assert.NotEmpty(t, listed[0].Hash)
}

// A gateway gets only the secrets its own deployed artifacts name: another gateway's or organization's handle is
// not found, even when the handle is right.
func TestGatewaySecretValueIsScopedToTheGateway(t *testing.T) {
	f := newSecretFixture(t, gwmodels.SecretDeliveryReference)
	ctx := context.Background()
	listed, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", false)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	handle := listed[0].Handle

	_, err = f.svc.GetGatewaySecretValue(ctx, "ou-1", "gw-2", handle)
	assert.ErrorIs(t, err, gwerr.ErrGatewaySecretNotFound)
	_, err = f.svc.GetGatewaySecretValue(ctx, "ou-2", "gw-1", handle)
	assert.ErrorIs(t, err, gwerr.ErrGatewaySecretNotFound)
	_, err = f.svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", "amp-unknown")
	assert.ErrorIs(t, err, gwerr.ErrGatewaySecretNotFound)
}

func TestInlineDeliveryOffersNoSecrets(t *testing.T) {
	f := newSecretFixture(t, gwmodels.SecretDeliveryInline)
	ctx := context.Background()

	got, err := f.svc.GetActiveAgentDeploymentByGateway(ctx, testArtifactID, "ou-1", "gw-1")
	require.NoError(t, err)
	assert.Contains(t, got[testArtifactID], "s3cret")

	listed, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", true)
	require.NoError(t, err)
	assert.Empty(t, listed)
	_, err = f.secrets.Get(ctx, "ou-1", gatewaySecretHandleKeyPath)
	assert.ErrorIs(t, err, modkit.ErrNotFound, "an inline gateway needs no handle key")
	handle, _ := gatewaySecretHandle(make([]byte, gatewaySecretHandleKeyBytes), f.ref, "s3cret")
	_, err = f.svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", handle)
	assert.ErrorIs(t, err, gwerr.ErrGatewaySecretNotFound)
}

// The gateway never refetches a handle it holds, so a new value must get a new handle; the old handle is then gone.
func TestRotatedSecretGetsANewHandle(t *testing.T) {
	f := newSecretFixture(t, gwmodels.SecretDeliveryReference)
	ctx := context.Background()
	before, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", false)
	require.NoError(t, err)

	require.NoError(t, f.secrets.Put(ctx, "ou-1", f.ref, "r0tated"))
	after, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", false)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.NotEqual(t, before[0].Handle, after[0].Handle)
	assert.NotEqual(t, before[0].Hash, after[0].Hash)

	_, err = f.svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", before[0].Handle)
	assert.ErrorIs(t, err, gwerr.ErrGatewaySecretNotFound)
	value, err := f.svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", after[0].Handle)
	require.NoError(t, err)
	assert.Equal(t, "r0tated", value)
}

func TestGatewaySecretHandleIsSafeForTheGateway(t *testing.T) {
	key := make([]byte, gatewaySecretHandleKeyBytes)
	handle, hash := gatewaySecretHandle(key, "mcp-proxies/x/endpoints/y/upstream-auth", "v")
	// The gateway puts the handle in a URL path and matches it with [^"\\]+.
	assert.Regexp(t, `^amp-[0-9a-f]{24}-[0-9a-f]{16}$`, handle)
	assert.Len(t, hash, 64)
	assert.NotContains(t, handle, "v")
}

// The handle is keyed: without the organization's key, it can't be recomputed from a guessed value.
func TestGatewaySecretHandleDependsOnTheKey(t *testing.T) {
	path := "llm-providers/x/upstream-auth"
	keyA := make([]byte, gatewaySecretHandleKeyBytes)
	keyB := make([]byte, gatewaySecretHandleKeyBytes)
	keyB[0] = 1
	handleA, hashA := gatewaySecretHandle(keyA, path, "v")
	handleB, hashB := gatewaySecretHandle(keyB, path, "v")
	assert.NotEqual(t, handleA, handleB)
	assert.NotEqual(t, hashA, hashB)
	again, _ := gatewaySecretHandle(keyA, path, "v")
	assert.Equal(t, handleA, again)
}

// The organization's handle key is created once, in its secret store, and reused.
func TestGatewaySecretHandleKeyIsCreatedOnceAndKept(t *testing.T) {
	f := newSecretFixture(t, gwmodels.SecretDeliveryReference)
	ctx := context.Background()
	first, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", false)
	require.NoError(t, err)
	stored, err := f.secrets.Get(ctx, "ou-1", gatewaySecretHandleKeyPath)
	require.NoError(t, err)
	assert.Len(t, stored, 2*gatewaySecretHandleKeyBytes)

	second, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", false)
	require.NoError(t, err)
	assert.Equal(t, first[0].Handle, second[0].Handle)
	kept, err := f.secrets.Get(ctx, "ou-1", gatewaySecretHandleKeyPath)
	require.NoError(t, err)
	assert.Equal(t, stored, kept)
}

func TestMalformedGatewaySecretHandleKeyIsRefused(t *testing.T) {
	f := newSecretFixture(t, gwmodels.SecretDeliveryReference)
	ctx := context.Background()
	require.NoError(t, f.secrets.Put(ctx, "ou-1", gatewaySecretHandleKeyPath, "not-hex"))
	_, err := f.svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", false)
	assert.ErrorContains(t, err, "malformed")
}

func TestValidateSecretDeliveryProperty(t *testing.T) {
	assert.NoError(t, validateSecretDeliveryProperty(nil))
	assert.NoError(t, validateSecretDeliveryProperty(map[string]interface{}{"other": 1}))
	assert.NoError(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": "inline"}))
	assert.NoError(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": "reference"}))
	assert.Error(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": "vault"}))
	assert.Error(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": true}))
}

// trackedSecrets is a secret store that counts reads per path and fails reads of the paths in failing.
type trackedSecrets struct {
	*gwtest.Secrets
	reads   map[string]int
	failing map[string]error
}

func (s *trackedSecrets) Get(ctx context.Context, orgID, path string) (string, error) {
	s.reads[path]++
	if err := s.failing[path]; err != nil {
		return "", err
	}
	return s.Secrets.Get(ctx, orgID, path)
}

func (s *trackedSecrets) In(*gorm.DB) modkit.Secrets { return s }

// newMultiSecretFixture is gw-1 in ou-1 taking secrets by reference, with one deployed A2A agent per ref. Only the
// refs in stored have a secret; the others name one that was removed after the agent was deployed.
func newMultiSecretFixture(t *testing.T, refs []string, stored map[string]string) (*GatewayInternalAPIService, *trackedSecrets) {
	t.Helper()
	secrets := &trackedSecrets{Secrets: gwtest.NewSecrets(), reads: map[string]int{}, failing: map[string]error{}}
	for ref, value := range stored {
		require.NoError(t, secrets.Put(context.Background(), "ou-1", ref, value))
	}
	var rows []gwmodels.GatewaySyncRow
	var contents []gwmodels.GatewayDeploymentContent
	for _, ref := range refs {
		in := proxiedA2AInput()
		in.UpstreamAuth.SecretRef = &ref
		yaml, err := generateA2AAgentDeploymentYAML(in)
		require.NoError(t, err)
		artifact := gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: gwmodels.KindAgent}
		id := uuid.New()
		rows = append(rows, gwmodels.GatewaySyncRow{ArtifactRef: artifact, DeploymentID: id.String(), State: gwmodels.GatewaySyncStateDeployed})
		contents = append(contents, gwmodels.GatewayDeploymentContent{ArtifactRef: artifact, DeploymentID: id, Content: []byte(yaml)})
	}
	repo := &gwrepomocks.DeploymentRepositoryMock{
		ListForGatewaySyncFunc: func(context.Context, string, string) ([]gwmodels.GatewaySyncRow, error) {
			return rows, nil
		},
		ListContentForGatewayFunc: func(context.Context, string, string, []uuid.UUID) ([]gwmodels.GatewayDeploymentContent, error) {
			return contents, nil
		},
	}
	svc := NewGatewayInternalAPIService(nil, nil, repo, nil, gatewayWithDelivery(gwmodels.SecretDeliveryReference), nil, secrets, gatewayKinds())
	return svc, secrets
}

// handleFor is the handle the gateway is sent for ref holding value.
func handleFor(t *testing.T, secrets *trackedSecrets, ref, value string) string {
	t.Helper()
	stored, err := secrets.Secrets.Get(context.Background(), "ou-1", gatewaySecretHandleKeyPath)
	require.NoError(t, err)
	key, err := hex.DecodeString(stored)
	require.NoError(t, err)
	handle, _ := gatewaySecretHandle(key, ref, value)
	return handle
}

// A deployed artifact whose credential was removed must not stop the gateway fetching or restoring the others.
func TestMissingSecretDoesNotBlockTheGatewaysOtherSecrets(t *testing.T) {
	healthy, removed := "a2a-proxies/1/upstream-auth", "a2a-proxies/2/upstream-auth"
	svc, secrets := newMultiSecretFixture(t, []string{healthy, removed}, map[string]string{healthy: "s3cret"})
	ctx := context.Background()

	listed, err := svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", true)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.NotNil(t, listed[0].Value)
	assert.Equal(t, "s3cret", *listed[0].Value)

	value, err := svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", listed[0].Handle)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", value)

	_, err = svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", handleFor(t, secrets, removed, "old"))
	assert.ErrorIs(t, err, gwerr.ErrGatewaySecretNotFound)
}

// Fetching one handle reads only that secret, so an unreadable secret elsewhere does not fail it and N fetches cost
// N reads, not N².
func TestGatewaySecretValueReadsOnlyTheNamedSecret(t *testing.T) {
	refs := []string{"a2a-proxies/1/upstream-auth", "a2a-proxies/2/upstream-auth", "a2a-proxies/3/upstream-auth"}
	stored := map[string]string{refs[0]: "one", refs[1]: "two", refs[2]: "three"}
	svc, secrets := newMultiSecretFixture(t, refs, stored)
	ctx := context.Background()
	_, err := svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", false)
	require.NoError(t, err)
	handle := handleFor(t, secrets, refs[1], "two")

	secrets.failing[refs[0]] = errors.New("vault unavailable")
	secrets.reads = map[string]int{}
	value, err := svc.GetGatewaySecretValue(ctx, "ou-1", "gw-1", handle)
	require.NoError(t, err)
	assert.Equal(t, "two", value)
	assert.Equal(t, map[string]int{gatewaySecretHandleKeyPath: 1, refs[1]: 1}, secrets.reads)

	// The list still fails on an error other than a missing secret: a partial list would hide an outage.
	_, err = svc.ListGatewaySecrets(ctx, "ou-1", "gw-1", true)
	assert.ErrorContains(t, err, "vault unavailable")
}

// lateKeySecrets is a secret store in which another replica creates the handle key just after this one found none:
// the first read of the key misses although the key is stored.
type lateKeySecrets struct {
	*gwtest.Secrets
	missed bool
}

func (s *lateKeySecrets) Get(ctx context.Context, orgID, path string) (string, error) {
	if path == gatewaySecretHandleKeyPath && !s.missed {
		s.missed = true
		return "", modkit.ErrNotFound
	}
	return s.Secrets.Get(ctx, orgID, path)
}

func (s *lateKeySecrets) In(*gorm.DB) modkit.Secrets { return s }

// A replica that finds no handle key while another is creating one must use the key that was stored, not overwrite
// it: the other replica may already have sent handles under it.
func TestGatewaySecretHandleKeyCreatedConcurrentlyIsNotReplaced(t *testing.T) {
	secrets := &lateKeySecrets{Secrets: gwtest.NewSecrets()}
	ctx := context.Background()
	winner := strings.Repeat("ab", gatewaySecretHandleKeyBytes)
	require.NoError(t, secrets.Put(ctx, "ou-1", gatewaySecretHandleKeyPath, winner))
	svc := NewGatewayInternalAPIService(nil, nil, nil, nil, nil, nil, secrets, gatewayKinds())

	key, err := svc.gatewaySecretHandleKey(ctx, "ou-1")
	require.NoError(t, err)
	require.True(t, secrets.missed)
	assert.Equal(t, winner, hex.EncodeToString(key))
	stored, err := secrets.Get(ctx, "ou-1", gatewaySecretHandleKeyPath)
	require.NoError(t, err)
	assert.Equal(t, winner, stored)
}

// A store that cannot create a key atomically is refused rather than risking two keys.
func TestGatewaySecretHandleKeyNeedsAtomicCreate(t *testing.T) {
	svc := NewGatewayInternalAPIService(nil, nil, nil, nil, nil, nil, putOnlySecrets{gwtest.NewSecrets()}, gatewayKinds())
	_, err := svc.gatewaySecretHandleKey(context.Background(), "ou-1")
	assert.ErrorContains(t, err, "atomically")
}

// putOnlySecrets hides gwtest.Secrets' Create.
type putOnlySecrets struct{ s *gwtest.Secrets }

func (p putOnlySecrets) Prepare(ctx context.Context, o, path, v string) (context.Context, error) {
	return p.s.Prepare(ctx, o, path, v)
}
func (p putOnlySecrets) Put(ctx context.Context, o, path, v string) error {
	return p.s.Put(ctx, o, path, v)
}
func (p putOnlySecrets) Get(ctx context.Context, o, path string) (string, error) {
	return p.s.Get(ctx, o, path)
}
func (p putOnlySecrets) Delete(ctx context.Context, o, path string) error {
	return p.s.Delete(ctx, o, path)
}
func (p putOnlySecrets) In(*gorm.DB) modkit.Secrets { return p }
