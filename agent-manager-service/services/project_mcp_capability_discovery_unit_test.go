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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// memDiscoveries is the discovery queue over the fixture's stored bindings, with the repository's token check.
type memDiscoveries struct {
	proxy func() *models.MCPProxy
	ouID  string
}

// state is the discovery state of the binding with artifact, and where to write it.
func (m *memDiscoveries) state(artifact uuid.UUID) (**models.MCPCapabilityDiscoveryState, bool) {
	p := m.proxy()
	for i := range p.Endpoints {
		for _, ee := range p.Endpoints[i].Environments {
			if ee.ArtifactUUID == artifact {
				return &p.Endpoints[i].Configuration.CapabilityDiscovery, true
			}
		}
	}
	return nil, false
}

func (m *memDiscoveries) WithTx(*gorm.DB) repositories.MCPCapabilityDiscoveryRepository { return m }

func (m *memDiscoveries) Enqueue(_ context.Context, artifact uuid.UUID) error {
	slot, ok := m.state(artifact)
	if !ok {
		return nil
	}
	now := time.Now()
	next := &models.MCPCapabilityDiscoveryState{
		Status: models.MCPCapabilityDiscoveryPending, NextAttemptAt: &now, Token: uuid.NewString(),
	}
	if *slot != nil {
		next.DiscoveredAt = (*slot).DiscoveredAt
	}
	*slot = next
	return nil
}

func (m *memDiscoveries) ClaimDue(_ context.Context, now time.Time, _ int) ([]models.MCPCapabilityDiscovery, error) {
	var due []models.MCPCapabilityDiscovery
	for _, e := range m.proxy().Endpoints {
		for _, ee := range e.Environments {
			slot, _ := m.state(ee.ArtifactUUID)
			d := *slot
			if d == nil || d.Status != models.MCPCapabilityDiscoveryPending || d.NextAttemptAt == nil || d.NextAttemptAt.After(now) {
				continue
			}
			lease := now.Add(repositories.MCPCapabilityDiscoveryClaimLease)
			d.NextAttemptAt = &lease
			due = append(due, models.MCPCapabilityDiscovery{MCPCapabilityDiscoveryState: *d, ArtifactUUID: ee.ArtifactUUID, OUID: m.ouID})
		}
	}
	return due, nil
}

func (m *memDiscoveries) update(read models.MCPCapabilityDiscovery, next *models.MCPCapabilityDiscoveryState) error {
	slot, ok := m.state(read.ArtifactUUID)
	if !ok || *slot == nil || (*slot).Token != read.Token {
		return repositories.ErrMCPCapabilityDiscoverySuperseded
	}
	*slot = next
	return nil
}

func (m *memDiscoveries) MarkAttemptFailed(_ context.Context, read models.MCPCapabilityDiscovery, lastErr string, next time.Time) error {
	state := read.MCPCapabilityDiscoveryState
	state.AttemptCount++
	state.LastError, state.NextAttemptAt = lastErr, &next
	return m.update(read, &state)
}

func (m *memDiscoveries) MarkFailed(_ context.Context, read models.MCPCapabilityDiscovery, lastErr string) error {
	state := read.MCPCapabilityDiscoveryState
	state.AttemptCount++
	state.Status, state.LastError, state.NextAttemptAt = models.MCPCapabilityDiscoveryFailed, lastErr, nil
	return m.update(read, &state)
}

func (m *memDiscoveries) Drop(_ context.Context, read models.MCPCapabilityDiscovery) error {
	return m.update(read, nil)
}

func (m *memDiscoveries) StoreDiscovered(
	_ context.Context, read models.MCPCapabilityDiscovery, capabilities *models.MCPProxyCapabilities, at time.Time,
) error {
	p := m.proxy()
	for i := range p.Endpoints {
		for _, ee := range p.Endpoints[i].Environments {
			if ee.ArtifactUUID != read.ArtifactUUID {
				continue
			}
			cfg := &p.Endpoints[i].Configuration
			if cfg.CapabilityDiscovery == nil || cfg.CapabilityDiscovery.Token != read.Token || callerGaveCapabilities(*cfg) {
				return repositories.ErrMCPCapabilityDiscoverySuperseded
			}
			state := read.MCPCapabilityDiscoveryState
			state.Status, state.NextAttemptAt, state.LastError, state.DiscoveredAt = models.MCPCapabilityDiscoveryDiscovered, nil, "", &at
			cfg.CapabilityDiscovery = &state
			cfg.Capabilities, cfg.CapabilitiesDiscovered = capabilities, capabilities != nil
			return nil
		}
	}
	return repositories.ErrMCPCapabilityDiscoverySuperseded
}

// probeCall is one call the fake prober got.
type probeCall struct{ url, header, value string }

// discoveryFixture is a deployed project binding, secured with an API key, with a discovery queue and a fake MCP
// server behind its gateway.
type discoveryFixture struct {
	*keyFixture
	queue   *memDiscoveries
	calls   []probeCall
	respond func(call probeCall) (*models.MCPServerInfoFetchResponse, error)
	recon   *projectMCPCapabilityDiscoveryService
}

var discoveredTools = []map[string]interface{}{
	{"name": "get_weather", "description": "Current weather"},
	{"name": "get_forecast"},
}

func newDiscoveryFixture(t *testing.T, req models.MCPProxyEnvironmentRequest) *discoveryFixture {
	t.Helper()
	f := &discoveryFixture{keyFixture: newKeyFixture(t, req)}
	f.queue = &memDiscoveries{proxy: func() *models.MCPProxy { return f.proxy }, ouID: testOrg}
	f.respond = func(probeCall) (*models.MCPServerInfoFetchResponse, error) {
		tools := discoveredTools
		return &models.MCPServerInfoFetchResponse{Tools: &tools}, nil
	}
	f.svc.discoveries = f.queue
	f.svc.base.repo.(*repomocks.MCPProxyRepositoryMock).GetByUUIDFunc = func(_ context.Context, id, _ string) (*models.MCPProxy, error) {
		if id != f.proxy.UUID.String() {
			return nil, utils.ErrMCPProxyNotFound
		}
		cp := *f.proxy
		cp.Endpoints = append([]models.MCPProxyEndpoint(nil), f.proxy.Endpoints...)
		return &cp, nil
	}
	f.svc.base.endpointRepo.(*repomocks.MCPProxyEndpointRepositoryMock).GetEndpointEnvByArtifactFunc = func(_ context.Context, id uuid.UUID) (*models.MCPProxyEndpointEnvironment, error) {
		for _, e := range f.proxy.Endpoints {
			for _, ee := range e.Environments {
				if ee.ArtifactUUID == id {
					return &ee, nil
				}
			}
		}
		return nil, gorm.ErrRecordNotFound
	}
	prober := func(_ context.Context, url, header, value string) (*models.MCPServerInfoFetchResponse, error) {
		call := probeCall{url: url, header: header, value: value}
		f.calls = append(f.calls, call)
		return f.respond(call)
	}
	f.recon = newProjectMCPCapabilityDiscoveryService(f.queue, f.svc, prober, discardLogger())
	f.recon.keyWait, f.recon.keyPoll = 50*time.Millisecond, time.Millisecond
	f.setDeployed(t)
	return f
}

// setDeployed acks the binding's deploy.
func (f *discoveryFixture) setDeployed(t *testing.T) {
	t.Helper()
	_, ee := f.binding()
	require.NotNil(t, ee)
	row, ok := f.status[ee.ArtifactUUID]
	require.True(t, ok, "the binding was deployed")
	row.Status = models.DeploymentStatusDeployed
}

func (f *discoveryFixture) enqueue(t *testing.T) {
	t.Helper()
	require.NoError(t, f.queue.Enqueue(context.Background(), uuid.MustParse(f.artifact())))
}

// row is the binding's discovery state as stored, with its binding.
func (f *discoveryFixture) row(t *testing.T) models.MCPCapabilityDiscovery {
	t.Helper()
	slot, ok := f.queue.state(uuid.MustParse(f.artifact()))
	require.True(t, ok)
	require.NotNil(t, *slot, "the binding has a discovery")
	return models.MCPCapabilityDiscovery{MCPCapabilityDiscoveryState: **slot, ArtifactUUID: uuid.MustParse(f.artifact()), OUID: testOrg}
}

// setRow overwrites the binding's stored discovery state.
func (f *discoveryFixture) setRow(t *testing.T, row models.MCPCapabilityDiscovery) {
	t.Helper()
	slot, ok := f.queue.state(row.ArtifactUUID)
	require.True(t, ok)
	state := row.MCPCapabilityDiscoveryState
	*slot = &state
}

func (f *discoveryFixture) get(t *testing.T) *models.MCPProxyEnvironmentResponse {
	t.Helper()
	resp, err := f.svc.GetEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	return resp
}

// A deployed binding's capabilities are listed through its gateway with an attempt key, stored as discovered, and the
// key is revoked; a later PUT without capabilities keeps them without a redeploy.
func TestCapabilityDiscovery_DiscoversThroughTheGatewayWithAnAttemptKey(t *testing.T) {
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{})
	f.enqueue(t)

	f.recon.RunOnce(context.Background())

	require.Len(t, f.calls, 1)
	call := f.calls[0]
	assert.Equal(t, "https://dev-gw.example.com/p1/weather/mcp", call.url, "the binding's public URL")
	assert.Equal(t, "X-API-Key", call.header)
	require.NotEmpty(t, call.value)
	assert.True(t, strings.HasPrefix(mcpDiscoveryKeyName, models.APIKeyTestKeyPrefix), "a name user keys cannot take")
	_, stillThere := f.keys[f.artifact()+"/"+mcpDiscoveryKeyName]
	assert.False(t, stillThere, "the attempt key is revoked after the attempt")

	resp := f.get(t)
	require.NotNil(t, resp.Capabilities)
	require.NotNil(t, resp.Capabilities.Tools)
	assert.Len(t, *resp.Capabilities.Tools, 2)
	assert.Equal(t, models.MCPCapabilitiesSourceDiscovered, resp.CapabilitiesSource)
	require.NotNil(t, resp.CapabilityDiscovery)
	assert.Equal(t, "Discovered", resp.CapabilityDiscovery.Status)
	assert.NotNil(t, resp.CapabilityDiscovery.DiscoveredAt)

	deploys := len(f.deployed)
	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.Capabilities, "a PUT without capabilities keeps the discovered ones")
	assert.Equal(t, models.MCPCapabilitiesSourceDiscovered, resp.CapabilitiesSource)
	assert.Equal(t, deploys, len(f.deployed), "and does not redeploy")
}

// The attempt key reaches the gateway a moment after it is issued: a 401 is retried within the attempt.
func TestCapabilityDiscovery_RetriesA401WhileTheKeyArrives(t *testing.T) {
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{})
	f.enqueue(t)
	base := f.respond
	f.respond = func(call probeCall) (*models.MCPServerInfoFetchResponse, error) {
		if len(f.calls) < 3 {
			return nil, utils.ErrMCPServerUnauthorized
		}
		return base(call)
	}

	f.recon.RunOnce(context.Background())

	assert.Len(t, f.calls, 3)
	assert.Equal(t, models.MCPCapabilityDiscoveryDiscovered, f.row(t).Status)
}

// A binding with security off is probed with no key.
func TestCapabilityDiscovery_OpenBindingIsProbedWithoutAKey(t *testing.T) {
	off := false
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{Enabled: &off}})
	f.enqueue(t)

	f.recon.RunOnce(context.Background())

	require.Len(t, f.calls, 1)
	assert.Empty(t, f.calls[0].header)
	assert.Empty(t, f.keys, "no key is issued")
	assert.Equal(t, models.MCPCapabilityDiscoveryDiscovered, f.row(t).Status)
}

// A binding not deployed yet, or an MCP server that fails, is retried with the reason recorded; at the attempt budget
// the row is failed.
func TestCapabilityDiscovery_RetriesThenFails(t *testing.T) {
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{})
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeploying
	f.enqueue(t)

	f.recon.RunOnce(context.Background())
	row := f.row(t)
	assert.Equal(t, models.MCPCapabilityDiscoveryPending, row.Status)
	assert.Equal(t, 1, row.AttemptCount)
	assert.Contains(t, row.LastError, "not deployed")
	assert.Empty(t, f.calls, "a binding not deployed is not probed")

	f.setDeployed(t)
	f.respond = func(probeCall) (*models.MCPServerInfoFetchResponse, error) {
		return nil, errors.New("connection refused")
	}
	row.AttemptCount = mcpDiscoveryAttemptBudget - 1
	now := time.Now()
	row.NextAttemptAt = &now
	f.setRow(t, row)

	f.recon.RunOnce(context.Background())
	row = f.row(t)
	assert.Equal(t, models.MCPCapabilityDiscoveryFailed, row.Status)
	assert.Contains(t, row.LastError, "connection refused")
	assert.Equal(t, "Failed", f.get(t).CapabilityDiscovery.Status)
	assert.Empty(t, f.keys, "a failed attempt revokes its key too")
}

// A binding whose caller gave capabilities is left alone: its row is dropped unprobed, and refresh is refused.
func TestCapabilityDiscovery_CallerCapabilitiesWin(t *testing.T) {
	tools := []map[string]interface{}{{"name": "from_the_caller"}}
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{Capabilities: &models.MCPProxyCapabilities{Tools: &tools}})
	f.enqueue(t)

	f.recon.RunOnce(context.Background())

	assert.Empty(t, f.calls)
	_, ee := f.binding()
	slot, _ := f.queue.state(ee.ArtifactUUID)
	assert.Nil(t, *slot, "the discovery is dropped")
	resp := f.get(t)
	assert.Equal(t, models.MCPCapabilitiesSourceCaller, resp.CapabilitiesSource)
	assert.Equal(t, "from_the_caller", (*resp.Capabilities.Tools)[0]["name"])

	_, err := f.svc.RefreshEnvironmentCapabilities(context.Background(), testOrg, bindingProjectUUID, bindingHandle, bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPCapabilitiesGivenByCaller)
}

// Refresh queues discovery with a fresh budget; a caller PUT with capabilities afterwards replaces discovered ones.
func TestCapabilityDiscovery_RefreshThenCallerOverride(t *testing.T) {
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{})

	resp, err := f.svc.RefreshEnvironmentCapabilities(context.Background(), testOrg, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	require.NotNil(t, resp.CapabilityDiscovery)
	assert.Equal(t, "Pending", resp.CapabilityDiscovery.Status)

	f.recon.RunOnce(context.Background())
	assert.Equal(t, models.MCPCapabilitiesSourceDiscovered, f.get(t).CapabilitiesSource)

	tools := []map[string]interface{}{{"name": "only_this"}}
	resp, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: &models.MCPProxyCapabilities{Tools: &tools}})
	require.NoError(t, err)
	assert.Equal(t, models.MCPCapabilitiesSourceCaller, resp.CapabilitiesSource)
	require.Len(t, *resp.Capabilities.Tools, 1)
	assert.Nil(t, resp.CapabilityDiscovery, "the earlier discovery no longer applies")

	_, err = f.svc.RefreshEnvironmentCapabilities(context.Background(), testOrg, bindingProjectUUID, "missing", bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
}

// A row queued again during an attempt is not overwritten by the attempt's outcome.
func TestCapabilityDiscovery_RequeueDuringAttemptWins(t *testing.T) {
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{})
	f.enqueue(t)
	base := f.respond
	f.respond = func(call probeCall) (*models.MCPServerInfoFetchResponse, error) {
		time.Sleep(time.Millisecond)
		f.enqueue(t)
		return base(call)
	}

	f.recon.RunOnce(context.Background())

	assert.Equal(t, models.MCPCapabilityDiscoveryPending, f.row(t).Status, "the newer queueing stands")
	assert.Nil(t, f.get(t).Capabilities, "the superseded attempt stored nothing")
}

// A binding PUT that changes the binding keeps a queued discovery's state: it is bookkeeping, not configuration.
func TestCapabilityDiscovery_PutKeepsQueuedState(t *testing.T) {
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{})
	f.enqueue(t)
	before := f.row(t)

	off := false
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{Enabled: &off}})
	require.NoError(t, err)

	after := f.row(t)
	assert.Equal(t, before.Token, after.Token, "the queued discovery survives the PUT")
	assert.Equal(t, models.MCPCapabilityDiscoveryPending, after.Status)
}

// A binding whose key is read from the query string fails discovery without issuing a key or probing: the key is
// never put in a URL, where a transport error would carry it into lastError and the logs.
func TestCapabilityDiscovery_QueryKeyBindingFailsWithoutAKeyInAURL(t *testing.T) {
	on := true
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{
		Enabled: &on, APIKey: &models.APIKeySecurity{Enabled: &on, Key: "api_key", In: "query"},
	}})
	f.enqueue(t)

	f.recon.RunOnce(context.Background())

	assert.Empty(t, f.calls, "not probed")
	assert.Empty(t, f.keys, "no key issued")
	row := f.row(t)
	assert.Equal(t, models.MCPCapabilityDiscoveryFailed, row.Status)
	assert.Contains(t, row.LastError, "query")
}

// An error that echoes the attempt key is stored, and so shown, with the key redacted.
func TestCapabilityDiscovery_ErrorsAreStoredWithTheKeyRedacted(t *testing.T) {
	f := newDiscoveryFixture(t, models.MCPProxyEnvironmentRequest{})
	f.enqueue(t)
	var leaked string
	f.respond = func(call probeCall) (*models.MCPServerInfoFetchResponse, error) {
		leaked = call.value
		return nil, fmt.Errorf("%w: upstream said bad key %s", utils.ErrURLUnreachable, call.value)
	}

	f.recon.RunOnce(context.Background())

	require.NotEmpty(t, leaked)
	row := f.row(t)
	assert.NotContains(t, row.LastError, leaked)
	assert.Contains(t, row.LastError, "[REDACTED]")
	assert.Contains(t, row.LastError, "upstream said bad key", "other detail is kept")
}
