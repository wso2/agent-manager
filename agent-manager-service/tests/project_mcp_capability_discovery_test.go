//go:build integration

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

package tests

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// TestProjectMCPCapabilityDiscoveryQueue drives a project binding's capability discovery queue against the database:
// the deploy ack queues it, GET shows it, the queue's claim, retry and superseded check, the refresh route, a caller
// that gives capabilities, and the row going with the binding.
func TestProjectMCPCapabilityDiscoveryQueue(t *testing.T) {
	suffix := uuid.NewString()[:8]
	project := "pd-" + suffix
	projectUUID := uuid.NewString()
	devEnv := "dev-" + suffix
	envUUID := uuid.New()

	oc := apitestutils.CreateMockOpenChoreoClient()
	baseProject := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := baseProject(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.Name, p.UUID = name, projectUUID
		return p, nil
	}
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
		if name != devEnv {
			return nil, utils.ErrNotFound
		}
		return &models.EnvironmentResponse{UUID: envUUID.String(), Name: name}, nil
	}
	oc.GetReleaseBindingEndpointServiceURLFunc = func(_ context.Context, _, _, component, _, _, _ string) (string, error) {
		return "http://" + component + ".dp-ns.svc.cluster.local:8080", nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))
	h := &projectProxyHarness{t: t, app: app, org: testOrgName}
	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())
	queue := repositories.NewMCPCapabilityDiscoveryRepository(gdb)

	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	kind := services.NewMCPProxyKind(repositories.NewGatewayRepo(gdb), deploymentRepo, queue)
	core := reconcile.NewCore(gdb, deploymentRepo, &lifecycleEventHub{}, reconcile.Kinds{kind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, testLifecycleLogger()).WithGateways(repositories.NewGatewayRepo(gdb))
	worker := reconcile.NewWorker(core, testLifecycleLogger())

	gatewayID := uuid.New()
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, "pd-gw-"+suffix, "pd-gw-"+suffix, "dev-gw.example.com", models.GatewayRoleBoth, ouID,
	).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, envUUID,
	).Error)
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})

	envPath := func(handle string) string { return "/" + handle + "/environments/" + devEnv }
	artifactOf := func(t *testing.T, handle string) uuid.UUID {
		t.Helper()
		var raw string
		require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ? AND b.environment_uuid = ?`,
			handle, ouID, envUUID).Row().Scan(&raw))
		return uuid.MustParse(raw)
	}
	// deploy binds handle with body and acks its deploy through the kind's real AfterLive.
	deploy := func(t *testing.T, handle string, body map[string]any) uuid.UUID {
		t.Helper()
		rr := h.do(http.MethodPut, project, "/"+handle, map[string]any{
			"name": "Proxy " + handle, "context": "/" + handle,
			"source": map[string]string{"componentName": handle, "endpointName": "mcp"},
		})
		require.Contains(t, []int{http.StatusCreated, http.StatusOK}, rr.Code, rr.Body.String())
		rr = h.do(http.MethodPut, project, envPath(handle), body)
		require.Contains(t, []int{http.StatusCreated, http.StatusOK}, rr.Code, rr.Body.String())
		require.NoError(t, worker.PlaceEnv(context.Background(), ouID, envUUID))
		artifact := artifactOf(t, handle)
		row, err := deploymentRepo.GetStatusRecord(context.Background(), models.Placement{
			ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifact, Kind: models.KindMCPMapping}, GatewayUUID: gatewayID,
		})
		require.NoError(t, err)
		require.NotNil(t, row.DeploymentID)
		applied, err := deploymentRepo.ApplyAck(context.Background(), gatewayID.String(), models.DeploymentAck{
			DeploymentID: row.DeploymentID.String(), ArtifactID: artifact.String(), ResourceType: "mcpproxy",
			Action: "deploy", Status: "success", PerformedAt: row.PerformedAt,
		}, core.AfterLive())
		require.NoError(t, err)
		require.True(t, applied)
		return artifact
	}
	discovery := func(t *testing.T, handle string) *models.MCPCapabilityDiscoveryStatus {
		t.Helper()
		rr := h.do(http.MethodGet, project, envPath(handle), nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		return decodeBinding(t, rr).CapabilityDiscovery
	}
	// claim claims due rows and returns the one for artifact.
	claim := func(t *testing.T, artifact uuid.UUID) models.MCPCapabilityDiscovery {
		t.Helper()
		rows, err := queue.ClaimDue(context.Background(), time.Now(), 100)
		require.NoError(t, err)
		for _, r := range rows {
			if r.ArtifactUUID == artifact {
				return r
			}
		}
		t.Fatalf("artifact %s was not claimed", artifact)
		return models.MCPCapabilityDiscovery{}
	}

	open := "open-" + suffix
	var openArtifact uuid.UUID
	t.Run("the deploy ack queues discovery, and GET shows it Pending", func(t *testing.T) {
		openArtifact = deploy(t, open, map[string]any{})
		got := discovery(t, open)
		require.NotNil(t, got)
		assert.Equal(t, "Pending", got.Status)
	})

	t.Run("a claim leases the row; a retry records the reason; a requeue supersedes the attempt", func(t *testing.T) {
		row := claim(t, openArtifact)
		rows, err := queue.ClaimDue(context.Background(), time.Now(), 100)
		require.NoError(t, err)
		for _, r := range rows {
			assert.NotEqual(t, openArtifact, r.ArtifactUUID, "a leased row is not claimed again")
		}

		require.NoError(t, queue.MarkAttemptFailed(context.Background(), row, "gateway said 503", time.Now().Add(-time.Second)))
		got := discovery(t, open)
		assert.Equal(t, "Pending", got.Status)
		assert.Equal(t, "gateway said 503", got.LastError)

		row = claim(t, openArtifact)
		assert.Equal(t, 1, row.AttemptCount)
		rr := h.do(http.MethodPost, project, envPath(open)+"/capabilities/refresh", nil)
		require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())
		require.ErrorIs(t, queue.MarkDiscovered(context.Background(), row, time.Now()),
			repositories.ErrMCPCapabilityDiscoverySuperseded, "the refresh supersedes the attempt in flight")

		row = claim(t, openArtifact)
		assert.Zero(t, row.AttemptCount, "a refresh gives a fresh budget")
		require.NoError(t, queue.MarkDiscovered(context.Background(), row, time.Now()))
		got = discovery(t, open)
		assert.Equal(t, "Discovered", got.Status)
		assert.NotNil(t, got.DiscoveredAt)
		assert.Empty(t, got.LastError)
	})

	t.Run("a binding whose caller gives capabilities is not queued, and refresh is 409", func(t *testing.T) {
		given := "given-" + suffix
		artifact := deploy(t, given, map[string]any{"capabilities": map[string]any{"tools": []map[string]any{{"name": "t1"}}}})
		_, err := queue.Get(context.Background(), ouID, artifact)
		require.ErrorIs(t, err, repositories.ErrMCPCapabilityDiscoveryNotFound)
		rr := h.do(http.MethodGet, project, envPath(given), nil)
		b := decodeBinding(t, rr)
		assert.Equal(t, models.MCPCapabilitiesSourceCaller, b.CapabilitiesSource)
		assert.Nil(t, b.CapabilityDiscovery)

		rr = h.do(http.MethodPost, project, envPath(given)+"/capabilities/refresh", nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeCapabilitiesGivenByCaller, errorCode(t, rr))
	})

	t.Run("refresh on an unbound environment is 404", func(t *testing.T) {
		rr := h.do(http.MethodPost, project, "/missing-"+suffix+"/environments/"+devEnv+"/capabilities/refresh", nil)
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})

	t.Run("the row goes with the binding", func(t *testing.T) {
		rr := h.do(http.MethodDelete, project, envPath(open), nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_capability_discoveries WHERE artifact_uuid = ?`, openArtifact).Scan(&n).Error)
		assert.Zero(t, n)
	})
}
