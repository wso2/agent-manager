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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testHealthChecksOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

func healthChecksAgentsURL() string {
	return fmt.Sprintf("/api/v1/orgs/%s/projects/my-project/agents", testHealthChecksOrgName)
}

// serveHealthChecksRequest sends a JSON request to the app and returns the recorded response.
func serveHealthChecksRequest(t *testing.T, app http.Handler, method, url string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		require.NoError(t, json.NewEncoder(buf).Encode(body))
	}
	req := httptest.NewRequest(method, url, buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	return rr
}

func healthChecksApp(t *testing.T, ocClient *clientmocks.OpenChoreoClientMock) http.Handler {
	t.Helper()
	return apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		OpenChoreoClient: ocClient,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
	}, jwtassertion.NewMockMiddleware(t))
}

// createA2AAgentBody is a create request for an A2A agent, with the given health checks.
func createA2AAgentBody(healthChecks any) map[string]any {
	body := map[string]any{
		"name":        fmt.Sprintf("a2a-agent-%s", uuid.New().String()[:5]),
		"displayName": "Trip Planner",
		"provisioning": map[string]any{
			"type": "internal",
			"repository": map[string]any{
				"url": "https://github.com/example/a2a-agent", "branch": "main", "appPath": "/",
			},
		},
		"agentType":      map[string]any{"type": "agent-api", "subType": "a2a-agent"},
		"build":          map[string]any{"type": "docker", "docker": map[string]any{"dockerfilePath": "/Dockerfile"}},
		"inputInterface": map[string]any{"type": "HTTP", "port": 9099},
	}
	if healthChecks != nil {
		body["healthChecks"] = healthChecks
	}
	return body
}

// mockCreatedAgent makes GetComponent return an internal agent, as it does once created.
func mockCreatedAgent(ocClient *clientmocks.OpenChoreoClientMock) {
	ocClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         uuid.New().String(),
			Name:         componentName,
			ProjectName:  projectName,
			Type:         models.AgentType{Type: "agent-api", SubType: "a2a-agent"},
			Provisioning: models.Provisioning{Type: "internal"},
			CreatedAt:    time.Now(),
		}, nil
	}
}

func TestCreateAgentHealthChecks(t *testing.T) {
	t.Run("an HTTP GET readiness check reaches OpenChoreo as sent", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		mockCreatedAgent(ocClient)
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodPost, healthChecksAgentsURL(), createA2AAgentBody(map[string]any{
			"readiness": map[string]any{"type": "http", "path": "/.well-known/agent-card.json"},
		}))

		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.CreateComponentCalls(), 1)
		checks := ocClient.CreateComponentCalls()[0].Req.HealthChecks
		require.NotNil(t, checks)
		require.NotNil(t, checks.Readiness)
		require.Equal(t, "http", *checks.Readiness.Type)
		require.Equal(t, "/.well-known/agent-card.json", *checks.Readiness.Path)
		// Fields not sent stay unset, so they render from the ComponentType's defaults.
		require.Nil(t, checks.Readiness.Enabled)
		require.Nil(t, checks.Readiness.PeriodSeconds)
		require.Nil(t, checks.Startup)
		require.Nil(t, checks.Liveness)
	})

	t.Run("no health checks leaves them to the defaults", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		mockCreatedAgent(ocClient)
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodPost, healthChecksAgentsURL(), createA2AAgentBody(nil))

		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.CreateComponentCalls(), 1)
		require.Nil(t, ocClient.CreateComponentCalls()[0].Req.HealthChecks)
	})

	t.Run("accepts a startup check sent in part, within an hour with the defaults", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		mockCreatedAgent(ocClient)
		app := healthChecksApp(t, ocClient)

		// The issue's example: 10s + 80 x 5s = 410s.
		rr := serveHealthChecksRequest(t, app, http.MethodPost, healthChecksAgentsURL(), createA2AAgentBody(map[string]any{
			"startup": map[string]any{"failureThreshold": 80},
		}))

		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.CreateComponentCalls(), 1)
	})

	for name, checks := range map[string]map[string]any{
		"an HTTP path without a leading slash": {
			"readiness": map[string]any{"type": "http", "path": "health"},
		},
		"an HTTP path with a space": {
			"readiness": map[string]any{"type": "http", "path": "/he alth"},
		},
		"a startup window over an hour": {
			// 10s + 400 x 10s = 4010s.
			"startup": map[string]any{"initialDelaySeconds": 10, "periodSeconds": 10, "failureThreshold": 400},
		},
		"a startup check sent in part, over an hour with the defaults": {
			// 10s + 60 x 3600s.
			"startup": map[string]any{"periodSeconds": 3600},
		},
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			ocClient := apitestutils.CreateMockOpenChoreoClient()
			app := healthChecksApp(t, ocClient)

			rr := serveHealthChecksRequest(t, app, http.MethodPost, healthChecksAgentsURL(), createA2AAgentBody(checks))

			require.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())
			require.Contains(t, rr.Body.String(), "Invalid health checks")
			require.Empty(t, ocClient.CreateComponentCalls())
		})
	}
}

func TestUpdateAgentBuildParametersHealthChecks(t *testing.T) {
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])
	url := fmt.Sprintf("%s/%s/build-parameters", healthChecksAgentsURL(), agentName)
	body := func(healthChecks any) map[string]any {
		b := map[string]any{
			"provisioning": map[string]any{
				"type": "internal",
				"repository": map[string]any{
					"url": "https://github.com/example/a2a-agent", "branch": "main", "appPath": "/",
				},
			},
			"agentType":      map[string]any{"type": "agent-api", "subType": "a2a-agent"},
			"build":          map[string]any{"type": "docker", "docker": map[string]any{"dockerfilePath": "/Dockerfile"}},
			"inputInterface": map[string]any{"type": "HTTP", "port": 9099},
		}
		if healthChecks != nil {
			b["healthChecks"] = healthChecks
		}
		return b
	}
	newClient := func() *clientmocks.OpenChoreoClientMock {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		mockCreatedAgent(ocClient)
		ocClient.UpdateComponentBuildParametersFunc = func(ctx context.Context, namespaceName, projectName, componentName string, req client.UpdateComponentBuildParametersRequest) error {
			return nil
		}
		return ocClient
	}

	t.Run("the health checks sent reach OpenChoreo", func(t *testing.T) {
		ocClient := newClient()
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodPut, url, body(map[string]any{
			"readiness": map[string]any{"type": "tcp"},
			"liveness":  map[string]any{"enabled": true, "type": "http", "path": "/live", "periodSeconds": 20},
		}))

		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.UpdateComponentBuildParametersCalls(), 1)
		checks := ocClient.UpdateComponentBuildParametersCalls()[0].Req.HealthChecks
		require.NotNil(t, checks)
		require.Equal(t, "tcp", *checks.Readiness.Type)
		require.True(t, *checks.Liveness.Enabled)
		require.Equal(t, "/live", *checks.Liveness.Path)
		require.EqualValues(t, 20, *checks.Liveness.PeriodSeconds)
	})

	t.Run("no health checks keeps the agent's", func(t *testing.T) {
		ocClient := newClient()
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodPut, url, body(nil))

		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.UpdateComponentBuildParametersCalls(), 1)
		require.Nil(t, ocClient.UpdateComponentBuildParametersCalls()[0].Req.HealthChecks)
	})

	t.Run("accepts a startup check sent in part", func(t *testing.T) {
		ocClient := newClient()
		app := healthChecksApp(t, ocClient)

		// 10s + 80 x 5s = 410s with the defaults.
		rr := serveHealthChecksRequest(t, app, http.MethodPut, url, body(map[string]any{
			"startup": map[string]any{"failureThreshold": 80},
		}))

		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.UpdateComponentBuildParametersCalls(), 1)
	})

	t.Run("rejects a startup check that waits over an hour with the defaults", func(t *testing.T) {
		ocClient := newClient()
		app := healthChecksApp(t, ocClient)

		// 10s + 800 x 5s = 4010s.
		rr := serveHealthChecksRequest(t, app, http.MethodPut, url, body(map[string]any{
			"startup": map[string]any{"failureThreshold": 800},
		}))

		require.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())
		require.Contains(t, rr.Body.String(), "Invalid health checks")
		require.Empty(t, ocClient.UpdateComponentBuildParametersCalls())
	})
}

// readinessOverHTTP is what GetEnvHealthChecks returns in these tests.
func readinessOverHTTP() *client.HealthChecks {
	enabled, checkType, path := true, "http", "/health"
	return &client.HealthChecks{Readiness: &client.HealthCheck{Enabled: &enabled, Type: &checkType, Path: &path}}
}

func TestGetAgentConfigurationsHealthChecks(t *testing.T) {
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])
	url := fmt.Sprintf("%s/%s/configurations?environment=development", healthChecksAgentsURL(), agentName)

	t.Run("shows the health checks of the release the environment runs", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvHealthChecksFunc = func(ctx context.Context, ouID, componentName, environment string) (*client.HealthChecks, error) {
			return readinessOverHTTP(), nil
		}
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodGet, url, nil)

		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.GetEnvHealthChecksCalls(), 1)
		require.Equal(t, "development", ocClient.GetEnvHealthChecksCalls()[0].Environment)
		var resp struct {
			Configurations struct {
				Probes map[string]map[string]any `json:"probes"`
			} `json:"configurations"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, "http", resp.Configurations.Probes["readiness"]["type"])
		require.Equal(t, "/health", resp.Configurations.Probes["readiness"]["path"])
	})

	t.Run("still returns the configurations when the health checks cannot be read", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvHealthChecksFunc = func(ctx context.Context, ouID, componentName, environment string) (*client.HealthChecks, error) {
			return nil, errors.New("openchoreo unavailable")
		}
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodGet, url, nil)

		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
		require.NotContains(t, rr.Body.String(), `"probes"`)
	})
}

func TestGetAgentHealthChecks(t *testing.T) {
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])
	url := fmt.Sprintf("%s/%s", healthChecksAgentsURL(), agentName)

	t.Run("shows the agent's current health checks", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvHealthChecksFunc = func(ctx context.Context, ouID, componentName, environment string) (*client.HealthChecks, error) {
			return readinessOverHTTP(), nil
		}
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodGet, url, nil)

		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.GetEnvHealthChecksCalls(), 1)
		require.Empty(t, ocClient.GetEnvHealthChecksCalls()[0].Environment, "no environment: the agent's current checks")
		var resp struct {
			HealthChecks map[string]map[string]any `json:"healthChecks"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, "http", resp.HealthChecks["readiness"]["type"])
	})

	t.Run("still returns the agent when the health checks cannot be read", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvHealthChecksFunc = func(ctx context.Context, ouID, componentName, environment string) (*client.HealthChecks, error) {
			return nil, errors.New("openchoreo unavailable")
		}
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodGet, url, nil)

		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
		require.NotContains(t, rr.Body.String(), `"healthChecks"`)
	})
}

// A catalog agent runs the published image, so it starts with the health checks
// published with the version, not the source agent's current ones, unless the
// request sends its own.
func TestCreateAgentFromKindHealthChecks(t *testing.T) {
	// seedKindVersion publishes a version with the given health checks (nil: a
	// version published before they were recorded).
	seedKindVersion := func(t *testing.T, healthChecks json.RawMessage) (kindName string) {
		gdb := db.DB(context.Background())
		kindName = fmt.Sprintf("test-kind-%s", uuid.New().String()[:5])
		kind := &models.AgentKind{
			ID:          uuid.New(),
			Name:        kindName,
			DisplayName: "Test Kind",
			OUID:        jwtassertion.MockOUID,
			ProjectName: testProjName,
			AgentName:   fmt.Sprintf("source-agent-%s", uuid.New().String()[:5]),
			Labels:      map[string]string{},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, gdb.Create(kind).Error)
		t.Cleanup(func() { gdb.Delete(kind) })
		kindVersion := &models.AgentKindVersion{
			ID:           uuid.New(),
			AgentKindID:  kind.ID,
			Version:      "v1",
			ImageId:      "sha256:test-kind-image",
			ConfigSchema: []models.KindConfigSchemaItem{},
			HealthChecks: healthChecks,
			CreatedAt:    time.Now(),
		}
		require.NoError(t, gdb.Create(kindVersion).Error)
		t.Cleanup(func() { gdb.Delete(kindVersion) })
		return kindName
	}
	newClient := func() *clientmocks.OpenChoreoClientMock {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.CreateInternalAgentFromKindWorkloadFunc = func(ctx context.Context, ouID, projectName, componentName string, req client.InternalAgentFromKindWorkloadRequest) error {
			return nil
		}
		ocClient.EnsureReleaseAndBindingFunc = func(ctx context.Context, ouID, projectName, componentName, environment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, traitEnvConfigs, componentTypeConfigs map[string]interface{}) error {
			return nil
		}
		// The source agent has since moved readiness to TCP; agents from the
		// version must not follow.
		ocClient.GetEnvHealthChecksFunc = func(ctx context.Context, ouID, componentName, environment string) (*client.HealthChecks, error) {
			checkType := "tcp"
			return &client.HealthChecks{Readiness: &client.HealthCheck{Type: &checkType}}, nil
		}
		return ocClient
	}
	body := func(kindName string, healthChecks any) map[string]any {
		b := map[string]any{
			"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
			"displayName": "Kind Agent",
			"provisioning": map[string]any{
				"type":      "internal",
				"agentKind": map[string]any{"name": kindName, "version": "v1"},
			},
		}
		if healthChecks != nil {
			b["healthChecks"] = healthChecks
		}
		return b
	}
	url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
	published := json.RawMessage(`{"readiness": {"enabled": true, "type": "http", "path": "/health"}}`)

	t.Run("starts with the health checks published with the version", func(t *testing.T) {
		kindName := seedKindVersion(t, published)
		ocClient := newClient()
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodPost, url, body(kindName, nil))

		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
		require.Empty(t, ocClient.GetEnvHealthChecksCalls(), "not the source agent's current health checks")
		require.Len(t, ocClient.CreateComponentCalls(), 1)
		require.Equal(t, readinessOverHTTP(), ocClient.CreateComponentCalls()[0].Req.HealthChecks)
	})

	t.Run("a version published before health checks were recorded gets the defaults", func(t *testing.T) {
		kindName := seedKindVersion(t, nil)
		ocClient := newClient()
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodPost, url, body(kindName, nil))

		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.CreateComponentCalls(), 1)
		require.Nil(t, ocClient.CreateComponentCalls()[0].Req.HealthChecks)
	})

	t.Run("health checks sent replace the published ones", func(t *testing.T) {
		kindName := seedKindVersion(t, published)
		ocClient := newClient()
		app := healthChecksApp(t, ocClient)

		rr := serveHealthChecksRequest(t, app, http.MethodPost, url, body(kindName, map[string]any{
			"readiness": map[string]any{"type": "tcp"},
		}))

		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
		require.Len(t, ocClient.CreateComponentCalls(), 1)
		checks := ocClient.CreateComponentCalls()[0].Req.HealthChecks
		require.Equal(t, "tcp", *checks.Readiness.Type)
		require.Nil(t, checks.Readiness.Path)
	})
}
