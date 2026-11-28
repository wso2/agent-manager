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
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

func syncRow(kind, handle, state, deploymentID string, deployedAt time.Time) gwmodels.GatewaySyncRow {
	return gwmodels.GatewaySyncRow{
		ArtifactRef:  gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: kind, Handle: handle},
		DeploymentID: deploymentID,
		State:        state,
		DeployedAt:   deployedAt,
	}
}

func TestListGatewayDeploymentsNamesEachArtifactAsTheGatewayStoresIt(t *testing.T) {
	deployedAt := gwmodels.NewPerformedAt(time.Now())
	provider := syncRow(gwmodels.KindLLMProvider, "provider-handle", gwmodels.GatewaySyncStateDeployed, uuid.NewString(), deployedAt)
	proxy := syncRow(gwmodels.KindLLMProxy, "proxy-handle", gwmodels.GatewaySyncStateDeployed, uuid.NewString(), deployedAt)
	mapping := syncRow(gwmodels.KindMCPMapping, "mapping", gwmodels.GatewaySyncStateUndeployed, "", deployedAt)
	mcpProxy := syncRow(gwmodels.KindMCPProxy, "mcp", gwmodels.GatewaySyncStateDeployed, uuid.NewString(), deployedAt)
	agent := syncRow(gwmodels.KindAgent, "p/a/env", gwmodels.GatewaySyncStateUndeployed, "", deployedAt)
	restAPI := syncRow(gwmodels.KindWebSubAPI, "api", gwmodels.GatewaySyncStateDeployed, uuid.NewString(), deployedAt)
	repo := &gwrepomocks.DeploymentRepositoryMock{
		ListForGatewaySyncFunc: func(_ context.Context, ouID, gatewayID string) ([]gwmodels.GatewaySyncRow, error) {
			assert.Equal(t, "ou-1", ouID)
			assert.Equal(t, "gw-1", gatewayID)
			return []gwmodels.GatewaySyncRow{provider, proxy, mapping, mcpProxy, agent, restAPI}, nil
		},
	}
	svc := newGatewayInternalServiceWithDeployments(repo)

	got, err := svc.ListGatewayDeployments(context.Background(), "ou-1", "gw-1")
	require.NoError(t, err)

	want := []struct {
		row        gwmodels.GatewaySyncRow
		kind, id   string
		deployment string
	}{
		{provider, "LlmProvider", provider.ArtifactUUID.String(), provider.DeploymentID},
		{proxy, "LlmProxy", "proxy-handle", proxy.DeploymentID},
		{mapping, "Mcp", mapping.ArtifactUUID.String(), ""},
		{mcpProxy, "Mcp", mcpProxy.ArtifactUUID.String(), mcpProxy.DeploymentID},
		{agent, "Agent", agent.ArtifactUUID.String(), ""},
	}
	require.Len(t, got, len(want), "a kind AMS doesn't deploy to gateways is left out")
	etags := map[string]bool{}
	for i, w := range want {
		assert.Equal(t, w.id, got[i].ArtifactID)
		assert.Equal(t, w.kind, got[i].Kind)
		assert.Equal(t, w.row.State, got[i].State)
		assert.Equal(t, w.deployment, got[i].DeploymentID)
		assert.True(t, deployedAt.Equal(got[i].DeployedAt))
		require.NoError(t, uuid.Validate(got[i].Etag))
		etags[got[i].Etag] = true
	}
	assert.Len(t, etags, len(want), "the gateway uses the etag as an event id, so entries never share one")

	again, err := svc.ListGatewayDeployments(context.Background(), "ou-1", "gw-1")
	require.NoError(t, err)
	assert.Equal(t, got[0].Etag, again[0].Etag, "the etag is stable for the same deployment")
}

// stubGatewayArtifacts names every artifact as kind Stub under its handle.
type stubGatewayArtifacts struct{}

func (stubGatewayArtifacts) GatewayArtifact(ref gwmodels.ArtifactRef) (string, string, bool) {
	return "Stub", ref.Handle, true
}

// The sync routes name an artifact as its kind does.
func TestGatewaySyncRoutesAskTheKinds(t *testing.T) {
	row := syncRow(gwmodels.KindWebSubAPI, "the-handle", gwmodels.GatewaySyncStateDeployed, uuid.NewString(), time.Now())
	repo := &gwrepomocks.DeploymentRepositoryMock{
		ListForGatewaySyncFunc: func(context.Context, string, string) ([]gwmodels.GatewaySyncRow, error) {
			return []gwmodels.GatewaySyncRow{row}, nil
		},
	}
	svc := NewGatewayInternalAPIService(nil, nil, repo, nil, nil, nil, nil, stubGatewayArtifacts{})

	got, err := svc.ListGatewayDeployments(context.Background(), "ou-1", "gw-1")

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Stub", got[0].Kind)
	assert.Equal(t, "the-handle", got[0].ArtifactID)
}

func TestSyncEtagChangesWithTheDeploymentAndItsTime(t *testing.T) {
	at := gwmodels.NewPerformedAt(time.Now())
	base := syncEtag("artifact", "deployment", at)
	assert.NotEqual(t, base, syncEtag("artifact", "other", at))
	assert.NotEqual(t, base, syncEtag("artifact", "", at))
	assert.NotEqual(t, base, syncEtag("artifact", "deployment", at.Add(time.Millisecond)))
	assert.NotEqual(t, syncEtag("a", "", at), syncEtag("b", "", at))
}

func TestFetchDeploymentBatch(t *testing.T) {
	secretRef := "llm-providers/1/upstream-auth"
	secrets := gwtest.NewSecrets()
	require.NoError(t, secrets.Put(context.Background(), "ou-1", secretRef, "s3cret"))

	provider := gwmodels.GatewayDeploymentContent{
		ArtifactRef:  gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: gwmodels.KindLLMProvider, Handle: "provider"},
		DeploymentID: uuid.New(),
		Content:      []byte("spec:\n  upstream:\n    auth:\n      header: X-Key\n      secretRef: " + secretRef + "\n"),
	}
	proxy := gwmodels.GatewayDeploymentContent{
		ArtifactRef:  gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: gwmodels.KindLLMProxy, Handle: "proxy-handle"},
		DeploymentID: uuid.New(),
		Content:      []byte("spec:\n  provider:\n    auth:\n      secretRef: not-stored\n"),
	}
	mcp := gwmodels.GatewayDeploymentContent{
		ArtifactRef:  gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: gwmodels.KindMCPMapping, Handle: "mcp"},
		DeploymentID: uuid.New(),
		Content:      []byte("spec:\n  policies:\n  - params:\n      secretRef: " + secretRef + "\n"),
	}
	restAPI := gwmodels.GatewayDeploymentContent{
		ArtifactRef:  gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: gwmodels.KindWebSubAPI},
		DeploymentID: uuid.New(),
		Content:      []byte("kind: RestApi\n"),
	}
	repo := &gwrepomocks.DeploymentRepositoryMock{
		ListContentForGatewayFunc: func(_ context.Context, ouID, gatewayID string, ids []uuid.UUID) ([]gwmodels.GatewayDeploymentContent, error) {
			assert.Equal(t, "ou-1", ouID)
			assert.Equal(t, "gw-1", gatewayID)
			assert.Equal(t, []uuid.UUID{provider.DeploymentID, proxy.DeploymentID, mcp.DeploymentID, restAPI.DeploymentID}, ids,
				"ids that aren't deployment ids are skipped, including the empty id of an undeployed entry")
			return []gwmodels.GatewayDeploymentContent{provider, proxy, mcp, restAPI}, nil
		},
	}
	svc := NewGatewayInternalAPIService(nil, nil, repo, nil, gatewayWithDelivery(gwmodels.SecretDeliveryInline), nil, secrets, gatewayKinds())

	got, err := svc.FetchDeploymentBatch(context.Background(), "ou-1", "gw-1", []string{
		strings.ToUpper(provider.DeploymentID.String()), "", proxy.DeploymentID.String(), "not-a-uuid",
		mcp.DeploymentID.String(), restAPI.DeploymentID.String(),
	})
	require.NoError(t, err)

	require.Len(t, got, 2, "a deployment whose secret can't be resolved, or of a kind AMS doesn't deploy, is left out")
	assert.Equal(t, gwutil.BatchDeployment{
		DeploymentID: provider.DeploymentID.String(), Kind: "LlmProvider", ArtifactID: provider.ArtifactUUID.String(),
		Content: []byte("spec:\n    upstream:\n        auth:\n            header: X-Key\n            value: s3cret\n"),
	}, got[0])
	assert.Equal(t, mcp.DeploymentID.String(), got[1].DeploymentID)
	assert.Equal(t, "Mcp", got[1].Kind)
	assert.Contains(t, string(got[1].Content), "value: s3cret")
	assert.NotContains(t, string(got[1].Content), "secretRef")
}

func TestFetchDeploymentBatchWithNoDeploymentIDs(t *testing.T) {
	svc := newGatewayInternalServiceWithDeployments(&gwrepomocks.DeploymentRepositoryMock{})

	got, err := svc.FetchDeploymentBatch(context.Background(), "ou-1", "gw-1", []string{"", "x"})

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCheckArtifactsExist(t *testing.T) {
	provider := gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: gwmodels.KindLLMProvider, Handle: "provider"}
	proxy := gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: gwmodels.KindLLMProxy, Handle: "proxy-handle"}
	missing := uuid.New()
	artifacts := &gwrepomocks.ArtifactRepositoryMock{
		ListExistingFunc: func(_ context.Context, ouID string, uuids []uuid.UUID, handles []string) ([]gwmodels.ArtifactRef, error) {
			assert.Equal(t, "ou-1", ouID)
			assert.Equal(t, []uuid.UUID{provider.ArtifactUUID, provider.ArtifactUUID, missing}, uuids)
			assert.Equal(t, []string{
				strings.ToUpper(provider.ArtifactUUID.String()), provider.ArtifactUUID.String(), "proxy-handle",
				missing.String(), "gone-handle",
			}, handles, "any id may be an LLM proxy handle")
			return []gwmodels.ArtifactRef{provider, proxy}, nil
		},
	}
	svc := NewGatewayInternalAPIService(nil, nil, nil, artifacts, nil, nil, nil, gatewayKinds())

	got, err := svc.CheckArtifactsExist(context.Background(), "ou-1", []string{
		strings.ToUpper(provider.ArtifactUUID.String()), provider.ArtifactUUID.String(), "proxy-handle",
		missing.String(), "gone-handle",
	})
	require.NoError(t, err)

	assert.Equal(t, []gwmodels.ArtifactExistence{
		{ArtifactID: strings.ToUpper(provider.ArtifactUUID.String()), Exists: true},
		{ArtifactID: provider.ArtifactUUID.String(), Exists: true},
		{ArtifactID: "proxy-handle", Exists: true},
		{ArtifactID: missing.String(), Exists: false},
		{ArtifactID: "gone-handle", Exists: false},
	}, got, "each requested id is answered as the gateway sent it, in its order")
}
