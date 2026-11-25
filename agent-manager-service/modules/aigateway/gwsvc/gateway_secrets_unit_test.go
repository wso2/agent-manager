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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	handle, _ := gatewaySecretHandle("ou-1", f.ref, "s3cret")
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
	handle, hash := gatewaySecretHandle("ou-1", "mcp-proxies/x/endpoints/y/upstream-auth", "v")
	// The gateway puts the handle in a URL path and matches it with [^"\\]+.
	assert.Regexp(t, `^amp-[0-9a-f]{24}-[0-9a-f]{16}$`, handle)
	assert.Len(t, hash, 64)
	assert.NotContains(t, handle, "v")
}

func TestValidateSecretDeliveryProperty(t *testing.T) {
	assert.NoError(t, validateSecretDeliveryProperty(nil))
	assert.NoError(t, validateSecretDeliveryProperty(map[string]interface{}{"other": 1}))
	assert.NoError(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": "inline"}))
	assert.NoError(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": "reference"}))
	assert.Error(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": "vault"}))
	assert.Error(t, validateSecretDeliveryProperty(map[string]interface{}{"secretDelivery": true}))
}
