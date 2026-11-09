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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// boundFixture is a project proxy at /weather with a Deployed binding on the fixture's gateway.
func boundFixture(t *testing.T) *bindingFixture {
	t.Helper()
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	f.gatewayLocks, f.rerenders = nil, 0
	return f
}

func (f *bindingFixture) upsertProxy(t *testing.T, name, ctxPath string) error {
	t.Helper()
	_, _, err := f.svc.Upsert(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle,
		models.ProjectMCPProxyRequest{
			Name: name, Version: "v1", Context: ctxPath,
			Source: &models.MCPProxySource{ComponentName: "weather", EndpointName: "mcp"},
		})
	return err
}

func TestUpsertProxy_ContextChangeIntoAClash_409NothingWritten(t *testing.T) {
	f := boundFixture(t)
	f.others[uuid.New()] = []byte("spec:\n  context: /p1/forecast/v2\n")

	err := f.upsertProxy(t, "Weather", "/forecast")

	require.ErrorIs(t, err, utils.ErrMCPContextInUse)
	assert.Zero(t, f.proxyUpdates, "the proxy row is untouched")
	assert.Zero(t, f.rerenders, "no binding is re-rendered")
	assert.Equal(t, "/weather", *f.proxy.Configuration.Context)
	assert.Equal(t, []string{f.gateway.UUID.String()}, f.gatewayLocks, "the bound gateway is locked for the check")
}

func TestUpsertProxy_ContextChangeWithNoClash_Rerenders(t *testing.T) {
	f := boundFixture(t)
	f.others[uuid.New()] = []byte("spec:\n  context: /p1/forecast-v2\n")

	require.NoError(t, f.upsertProxy(t, "Weather", "/forecast"))

	assert.Equal(t, 1, f.proxyUpdates)
	assert.Equal(t, 1, f.rerenders, "the binding is re-rendered at the new context")
	assert.Equal(t, "/forecast", *f.proxy.Configuration.Context)
}

func TestUpsertProxy_UnchangedContext_NotChecked(t *testing.T) {
	f := boundFixture(t)
	// An overlap that is already there doesn't stop a change that leaves the context alone.
	f.others[uuid.New()] = []byte("spec:\n  context: /p1/weather/tools\n")

	require.NoError(t, f.upsertProxy(t, "Weather renamed", "/weather"))

	assert.Equal(t, 1, f.proxyUpdates)
	assert.Empty(t, f.gatewayLocks, "no gateway is locked or checked")
}
