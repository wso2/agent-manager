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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// stubAgentService answers the agent service methods a test sets; any other
// method panics on the nil embedded interface.
type stubAgentService struct {
	services.AgentManagerService
	get        func(ouID, proj, agent string) (*models.AgentResponse, error)
	list       func(ouID, proj string, labels map[string]string, limit, offset int32) ([]*models.AgentResponse, int32, error)
	create     func(ouID, proj string, req *spec.CreateAgentRequest) error
	update     func(ouID, proj, agent string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error)
	del        func(ouID, proj, agent string) error
	genName    func(ouID string, req spec.ResourceNameRequest) (string, error)
	identity   func() ([]models.AgentIdentityEnvironmentView, error)
	regenerate func(env string) (*models.AgentRegenerateSecretResponse, error)
	revoke     func(env string) (models.AgentRevokeSecretResponse, error)
	provision  func(env string) (models.AgentIdentityEnvironmentView, bool, error)
	retry      func(env string) (models.AgentIdentityEnvironmentView, error)
	roles      func(env string) ([]thundersvc.ThunderRole, error)
	groups     func(env string) ([]thundersvc.ThunderGroup, error)
}

func (s *stubAgentService) GetAgent(_ context.Context, ouID, proj, agent string) (*models.AgentResponse, error) {
	return s.get(ouID, proj, agent)
}

func (s *stubAgentService) ListAgents(_ context.Context, ouID, proj string, labels map[string]string, limit, offset int32) ([]*models.AgentResponse, int32, error) {
	return s.list(ouID, proj, labels, limit, offset)
}

func (s *stubAgentService) CreateAgent(_ context.Context, ouID, proj string, req *spec.CreateAgentRequest) error {
	return s.create(ouID, proj, req)
}

func (s *stubAgentService) UpdateAgentBasicInfo(_ context.Context, ouID, proj, agent string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
	return s.update(ouID, proj, agent, req)
}

func (s *stubAgentService) DeleteAgent(_ context.Context, ouID, proj, agent string) error {
	return s.del(ouID, proj, agent)
}

func (s *stubAgentService) GenerateName(_ context.Context, ouID string, req spec.ResourceNameRequest) (string, error) {
	return s.genName(ouID, req)
}

func (s *stubAgentService) GetAgentIdentity(context.Context, string, string, string) ([]models.AgentIdentityEnvironmentView, error) {
	return s.identity()
}

func (s *stubAgentService) RegenerateAgentIdentitySecret(_ context.Context, _, _, _, env string) (*models.AgentRegenerateSecretResponse, error) {
	return s.regenerate(env)
}

func (s *stubAgentService) RevokeAgentIdentitySecret(_ context.Context, _, _, _, env string) (models.AgentRevokeSecretResponse, error) {
	return s.revoke(env)
}

func (s *stubAgentService) ProvisionAgentIdentity(_ context.Context, _, _, _, env string) (models.AgentIdentityEnvironmentView, bool, error) {
	return s.provision(env)
}

func (s *stubAgentService) RetryAgentIdentityProvisioning(_ context.Context, _, _, _, env string) (models.AgentIdentityEnvironmentView, error) {
	return s.retry(env)
}

func (s *stubAgentService) GetAgentRoles(_ context.Context, _, _, _, env string) ([]thundersvc.ThunderRole, error) {
	return s.roles(env)
}

func (s *stubAgentService) GetAgentGroups(_ context.Context, _, _, _, env string) ([]thundersvc.ThunderGroup, error) {
	return s.groups(env)
}

type recordingCleaner struct {
	agents []string
}

func (c *recordingCleaner) RemoveForProject(context.Context, string, string) {}

func (c *recordingCleaner) RemoveForAgent(_ context.Context, ouID, proj, agent string) {
	c.agents = append(c.agents, ouID+"/"+proj+"/"+agent)
}

type agentCall struct {
	method, url, body string
	rec               audit.Recorder
	// noProject leaves out the project query parameter every other call carries.
	noProject bool
}

func serveAgent(t *testing.T, svc services.AgentManagerService, cleaner services.EventsCleaner, h func(AgentController) http.HandlerFunc, c agentCall) *httptest.ResponseRecorder {
	t.Helper()
	url := c.url
	if !c.noProject && !strings.Contains(url, "project=") {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += sep + "project=proj"
	}
	req := httptest.NewRequest(c.method, url, strings.NewReader(c.body))
	req.SetPathValue(utils.PathParamAgentName, "agent")
	ctx := middleware.WithResolvedOrg(req.Context(), orgctx.ResolvedOrg{OUID: callerOU, OuHandle: "acme"})
	if c.rec == nil {
		c.rec = audit.NewNoopRecorder()
	}
	ctx = audit.WithRecorder(ctx, c.rec)
	w := httptest.NewRecorder()
	h(NewAgentController(svc, cleaner))(w, req.WithContext(ctx))
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) httpx.ErrorResponse {
	t.Helper()
	var body httpx.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body
}

// HandleCommonErrors is the one place the agent, project and token routes turn
// a service error into a status; each sentinel keeps its status and code even
// when wrapped.
func TestHandleCommonErrors_MapsEverySentinel(t *testing.T) {
	cases := []struct {
		err     error
		status  int
		code    string
		message string
	}{
		{utils.ErrOrganizationNotFound, http.StatusNotFound, utils.ErrCodeOrganizationNotFound, "Organization not found"},
		{utils.ErrProjectNotFound, http.StatusNotFound, utils.ErrCodeProjectNotFound, "Project not found"},
		{utils.ErrAgentNotFound, http.StatusNotFound, utils.ErrCodeAgentNotFound, "Agent not found"},
		{utils.ErrEnvironmentNotFound, http.StatusNotFound, utils.ErrCodeEnvironmentNotFound, "Environment not found"},
		{utils.ErrAgentAlreadyExists, http.StatusConflict, utils.ErrCodeAgentAlreadyExists, "Agent already exists"},
		{utils.ErrOrphanedAgentConfigsExist, http.StatusConflict, httpx.ErrCodeConflict, "Configurations from a deleted agent with this name still exist"},
		{utils.ErrProjectAlreadyExists, http.StatusConflict, utils.ErrCodeProjectAlreadyExists, "Project already exists"},
		{utils.ErrProjectHasAssociatedAgents, http.StatusConflict, httpx.ErrCodeConflict, "Project has associated agents"},
		{utils.ErrSecretPathConflict, http.StatusConflict, httpx.ErrCodeConflict, "Secret path conflict"},
		{utils.ErrResourceBeingDeleted, http.StatusConflict, httpx.ErrCodeConflict, "A resource with this name is still being deleted"},
		{httpx.ErrConflict, http.StatusConflict, httpx.ErrCodeConflict, "Conflict"},
		{httpx.ErrInvalidInput, http.StatusBadRequest, httpx.ErrCodeValidation, "Invalid input provided"},
		{utils.ErrImmutableFieldChange, http.StatusBadRequest, utils.ErrCodeImmutableField, "Cannot modify immutable field"},
		{httpx.ErrBadRequest, http.StatusBadRequest, httpx.ErrCodeBadRequest, "Bad request"},
		{utils.ErrEnvironmentHasNoGateway, http.StatusConflict, utils.ErrCodeEnvironmentHasNoGateway, "Environment has no ingress gateway"},
		{httpx.ErrUnauthorized, http.StatusUnauthorized, httpx.ErrCodeUnauthorized, "Unauthorized"},
		{httpx.ErrForbidden, http.StatusForbidden, httpx.ErrCodeForbidden, "Forbidden"},
		{httpx.ErrServiceUnavailable, http.StatusServiceUnavailable, httpx.ErrCodeServiceUnavailable, "Service temporarily unavailable"},
		{httpx.ErrNotImplemented, http.StatusNotImplemented, httpx.ErrCodeNotImplemented, ""},
		{errThunder, http.StatusInternalServerError, httpx.ErrCodeInternalError, "fallback message"},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			w := httptest.NewRecorder()
			HandleCommonErrors(w, fmt.Errorf("context: %w", tc.err), "fallback message")
			assert.Equal(t, tc.status, w.Code)
			body := decodeError(t, w)
			assert.Equal(t, tc.code, body.Code)
			if tc.message != "" {
				assert.Equal(t, tc.message, body.Message)
			}
		})
	}

	t.Run("a 500 never leaks the internal error", func(t *testing.T) {
		w := httptest.NewRecorder()
		HandleCommonErrors(w, fmt.Errorf("dial tcp 10.0.0.5:5432: connection refused"), "Failed to get agent")
		assert.NotContains(t, w.Body.String(), "10.0.0.5")
	})

	t.Run("a module hook's rejection keeps its status and message", func(t *testing.T) {
		w := httptest.NewRecorder()
		HandleCommonErrors(w, fmt.Errorf("hook: %w", &modkit.Error{Status: http.StatusUnprocessableEntity, Message: "gateway says no"}), "fallback")
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Equal(t, "gateway says no", decodeError(t, w).Message)
	})
}

func agentModel(name string) *models.AgentResponse {
	return &models.AgentResponse{
		UUID: "uuid-" + name, Name: name, DisplayName: "Agent " + name, ProjectName: "proj",
		CreatedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Provisioning: models.Provisioning{Type: string(utils.ExternalAgent)},
		Type:         models.AgentType{Type: "external-agent-api"},
		Labels:       map[string]string{"team": "a"},
	}
}

// agentDoc is an Agent document body for agent in project proj with the given spec.
func agentDoc(name, spec string) string {
	return `{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"Agent","metadata":{"name":"` + name +
		`","project":"proj"},"spec":` + spec + `}`
}

func TestAgentController_GetAgent(t *testing.T) {
	get := func(c AgentController) http.HandlerFunc { return c.GetAgent }
	svc := &stubAgentService{get: func(ouID, proj, agent string) (*models.AgentResponse, error) {
		assert.Equal(t, callerOU, ouID, "the service is addressed by the resolved org's OU")
		assert.Equal(t, "proj", proj)
		assert.Equal(t, "agent", agent)
		return agentModel(agent), nil
	}}
	w := serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x"})
	require.Equal(t, http.StatusOK, w.Code)
	var resp spec.AgentDocument
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ai-control-plane.choreolabs.com/v1", resp.ApiVersion)
	assert.Equal(t, "Agent", resp.Kind)
	assert.Equal(t, "agent", resp.Metadata.Name)
	assert.Equal(t, "proj", resp.Metadata.Project)
	assert.Equal(t, "uuid-agent", resp.Metadata.GetUid())
	assert.Equal(t, "Agent agent", resp.Spec.DisplayName)
	assert.Equal(t, "external", resp.Spec.Provisioning.Type)
	assert.Equal(t, map[string]string{"team": "a"}, resp.Spec.GetLabels())

	w = serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x", noProject: true})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "project query parameter is required", decodeError(t, w).Message)

	svc.get = func(string, string, string) (*models.AgentResponse, error) { return nil, utils.ErrAgentNotFound }
	w = serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x"})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, utils.ErrCodeAgentNotFound, decodeError(t, w).Code)
}

func TestAgentController_ListAgents(t *testing.T) {
	list := func(c AgentController) http.HandlerFunc { return c.ListAgents }

	for _, tc := range []struct{ query, msg string }{
		{"limit=0", "Invalid limit parameter: must be between 1 and 500"},
		{"limit=501", "Invalid limit parameter: must be between 1 and 500"},
		{"limit=abc", "Invalid limit parameter: must be between 1 and 500"},
		{"offset=-1", "Invalid offset parameter: must be 0 or greater"},
		{"offset=x", "Invalid offset parameter: must be 0 or greater"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			w := serveAgent(t, &stubAgentService{}, nil, list, agentCall{method: http.MethodGet, url: "/x?" + tc.query})
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.msg, decodeError(t, w).Message)
		})
	}

	t.Run("a malformed label selector is a validation error", func(t *testing.T) {
		w := serveAgent(t, &stubAgentService{}, nil, list, agentCall{method: http.MethodGet, url: "/x?label=novalue"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, httpx.ErrCodeValidation, decodeError(t, w).Code)
	})

	t.Run("defaults, labels and the page envelope", func(t *testing.T) {
		svc := &stubAgentService{list: func(ouID, proj string, labels map[string]string, limit, offset int32) ([]*models.AgentResponse, int32, error) {
			assert.Equal(t, callerOU, ouID)
			assert.Equal(t, map[string]string{"team": "a"}, labels)
			assert.Equal(t, int32(utils.DefaultLimit), limit)
			assert.Equal(t, int32(0), offset)
			return []*models.AgentResponse{agentModel("a1"), agentModel("a2")}, 7, nil
		}}
		w := serveAgent(t, svc, nil, list, agentCall{method: http.MethodGet, url: "/x?label=team:a"})
		require.Equal(t, http.StatusOK, w.Code)
		var resp spec.AgentListResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, int32(2), resp.Count)
		assert.Equal(t, int32(7), resp.Pagination.GetTotal())
		assert.Equal(t, int32(utils.DefaultLimit), resp.Pagination.GetLimit())
		require.Len(t, resp.List, 2)
		assert.Equal(t, "a2", resp.List[1].Metadata.Name)
	})

	t.Run("without a project the list spans the org", func(t *testing.T) {
		svc := &stubAgentService{list: func(_, proj string, _ map[string]string, _, _ int32) ([]*models.AgentResponse, int32, error) {
			assert.Empty(t, proj, "no project means every project")
			return nil, 0, nil
		}}
		w := serveAgent(t, svc, nil, list, agentCall{method: http.MethodGet, url: "/x?limit=500", noProject: true})
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"count":0,"list":[],"pagination":{"limit":500,"offset":0,"total":0}}`, w.Body.String(),
			"an empty page is [] on the wire, never null")
	})

	t.Run("an explicit page is passed through", func(t *testing.T) {
		svc := &stubAgentService{list: func(_, _ string, _ map[string]string, limit, offset int32) ([]*models.AgentResponse, int32, error) {
			assert.Equal(t, int32(10), limit)
			assert.Equal(t, int32(20), offset)
			return nil, 0, utils.ErrProjectNotFound
		}}
		w := serveAgent(t, svc, nil, list, agentCall{method: http.MethodGet, url: "/x?limit=10&offset=20"})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAgentController_CreateAgent(t *testing.T) {
	create := func(c AgentController) http.HandlerFunc { return c.CreateAgent }
	validSpec := `{"displayName":"My Agent","description":"d","provisioning":{"type":"external"},"agentType":{"type":"external-agent-api"},"labels":{"team":"a"}}`
	valid := agentDoc("my-agent", validSpec)

	for _, tc := range []struct {
		name, body string
		status     int
		msg        string
	}{
		{"malformed body", "{", http.StatusBadRequest, "Invalid request body"},
		{"wrong kind", strings.Replace(valid, `"kind":"Agent"`, `"kind":"LlmProvider"`, 1), http.StatusBadRequest, `kind must be "Agent" on this route, got "LlmProvider"`},
		{"wrong apiVersion", strings.Replace(valid, "choreolabs.com/v1", "choreolabs.com/v2", 1), http.StatusBadRequest, `apiVersion must be "ai-control-plane.choreolabs.com/v1", got "ai-control-plane.choreolabs.com/v2"`},
		{"missing project", strings.Replace(valid, `,"project":"proj"`, "", 1), http.StatusBadRequest, "metadata.project is required"},
		{"missing spec", `{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"Agent","metadata":{"name":"a","project":"proj"}}`, http.StatusBadRequest, "spec is required"},
		{"hosted agents are refused", agentDoc("a", `{"displayName":"x","provisioning":{"type":"internal"}}`), http.StatusNotImplemented, "Only external agents can be created in this deployment"},
		{"invalid name", agentDoc("Bad_Name", `{"displayName":"x","provisioning":{"type":"external"},"agentType":{"type":"external-agent-api"}}`), http.StatusBadRequest, "Agent name must contain only lowercase alphanumeric characters or '-'"},
		{"missing agent type", agentDoc("a", `{"displayName":"x","provisioning":{"type":"external"}}`), http.StatusBadRequest, "Agent type is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serveAgent(t, &stubAgentService{}, nil, create, agentCall{method: http.MethodPost, url: "/x", body: tc.body, noProject: true})
			assert.Equal(t, tc.status, w.Code)
			assert.Equal(t, tc.msg, decodeError(t, w).Message)
		})
	}

	t.Run("accepted with the document echoed", func(t *testing.T) {
		svc := &stubAgentService{create: func(ouID, proj string, req *spec.CreateAgentRequest) error {
			assert.Equal(t, callerOU, ouID)
			assert.Equal(t, "proj", proj, "the project comes from metadata.project")
			assert.Equal(t, "my-agent", req.Name)
			assert.Equal(t, "My Agent", req.DisplayName)
			assert.Equal(t, map[string]string{"team": "a"}, req.GetLabels())
			return nil
		}}
		w := serveAgent(t, svc, nil, create, agentCall{method: http.MethodPost, url: "/x", body: valid, noProject: true})
		require.Equal(t, http.StatusAccepted, w.Code)
		var resp spec.AgentDocument
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "my-agent", resp.Metadata.Name)
		assert.Equal(t, "proj", resp.Metadata.Project)
		assert.Equal(t, "d", resp.Spec.GetDescription())
		assert.Equal(t, "external-agent-api", resp.Spec.AgentType.Type)
		assert.Equal(t, map[string]string{"team": "a"}, resp.Spec.GetLabels())
		assert.False(t, resp.Metadata.GetCreationTimestamp().IsZero())
	})

	t.Run("an existing agent is a conflict", func(t *testing.T) {
		svc := &stubAgentService{create: func(string, string, *spec.CreateAgentRequest) error { return utils.ErrAgentAlreadyExists }}
		w := serveAgent(t, svc, nil, create, agentCall{method: http.MethodPost, url: "/x", body: valid, noProject: true})
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, utils.ErrCodeAgentAlreadyExists, decodeError(t, w).Code)
	})
}

func TestAgentController_UpdateAgentBasicInfo(t *testing.T) {
	update := func(c AgentController) http.HandlerFunc { return c.UpdateAgentBasicInfo }
	stored := func() *models.AgentResponse {
		a := agentModel("agent")
		a.Description = "kept"
		return a
	}
	put := func(svc *stubAgentService, body string) *httptest.ResponseRecorder {
		return serveAgent(t, svc, nil, update, agentCall{method: http.MethodPut, url: "/x", body: body})
	}
	newSvc := func(check func(req *spec.UpdateAgentBasicInfoRequest)) *stubAgentService {
		return &stubAgentService{
			get: func(string, string, string) (*models.AgentResponse, error) { return stored(), nil },
			update: func(_, _, agent string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
				if check != nil {
					check(req)
				}
				a := stored()
				a.DisplayName = req.DisplayName
				return a, nil
			},
		}
	}
	never := func(*spec.UpdateAgentBasicInfoRequest) { t.Error("the service must not be called") }

	for _, tc := range []struct{ name, body, msg string }{
		{"malformed body", "nope", "Invalid request body"},
		{"empty display name", agentDoc("agent", `{"displayName":""}`), "Agent display name cannot be empty"},
		{"another agent's name", agentDoc("other", `{"displayName":"x"}`), "metadata.name does not match the agent in the path"},
		{"another project", strings.Replace(agentDoc("agent", `{"displayName":"x"}`), `"project":"proj"`, `"project":"p2"`, 1), "metadata.project does not match the project of the agent"},
		{"changed provisioning", agentDoc("agent", `{"displayName":"x","provisioning":{"type":"internal"}}`), "spec.provisioning cannot be changed"},
		{"changed agent type", agentDoc("agent", `{"displayName":"x","agentType":{"type":"chat-api"}}`), "spec.agentType cannot be changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := put(newSvc(never), tc.body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.msg, decodeError(t, w).Message)
		})
	}

	t.Run("a read-edit-write round trip is accepted", func(t *testing.T) {
		svc := newSvc(func(req *spec.UpdateAgentBasicInfoRequest) {
			assert.Equal(t, "Renamed", req.DisplayName)
			assert.Equal(t, "", req.Description, "a declared description is applied, empty included")
			assert.Equal(t, map[string]string{}, req.GetLabels(), "declared empty labels clear them")
		})
		w := put(svc, agentDoc("agent", `{"displayName":"Renamed","description":"","provisioning":{"type":"external"},"agentType":{"type":"external-agent-api"},"labels":{}}`))
		require.Equal(t, http.StatusOK, w.Code)
		var resp spec.AgentDocument
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Renamed", resp.Spec.DisplayName)
	})

	t.Run("a create body's subtype is not a change", func(t *testing.T) {
		body := agentDoc("agent", `{"displayName":"x","agentType":{"type":"external-agent-api","subType":"a2a-agent"}}`)
		assert.Equal(t, http.StatusOK, put(newSvc(nil), body).Code, "no subtype is stored, so none can differ")
	})

	t.Run("an omitted field keeps its stored value", func(t *testing.T) {
		svc := newSvc(func(req *spec.UpdateAgentBasicInfoRequest) {
			assert.Equal(t, "kept", req.Description)
			assert.Nil(t, req.Labels, "absent labels are left alone")
			assert.Nil(t, req.Extensions, "absent extensions are left alone")
		})
		assert.Equal(t, http.StatusOK, put(svc, agentDoc("agent", `{"displayName":"Renamed"}`)).Code)
	})

	t.Run("a missing agent is 404", func(t *testing.T) {
		svc := &stubAgentService{get: func(string, string, string) (*models.AgentResponse, error) { return nil, utils.ErrAgentNotFound }}
		assert.Equal(t, http.StatusNotFound, put(svc, agentDoc("agent", `{"displayName":"x"}`)).Code)
	})

	t.Run("an immutable-field error from the service keeps its code", func(t *testing.T) {
		svc := newSvc(nil)
		svc.update = func(string, string, string, *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
			return nil, utils.ErrImmutableFieldChange
		}
		w := put(svc, agentDoc("agent", `{"displayName":"x"}`))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, utils.ErrCodeImmutableField, decodeError(t, w).Code)
	})
}

func TestAgentController_DeleteAgent(t *testing.T) {
	del := func(c AgentController) http.HandlerFunc { return c.DeleteAgent }

	t.Run("removes the agent's event subscriptions after deleting it", func(t *testing.T) {
		cleaner := &recordingCleaner{}
		svc := &stubAgentService{del: func(string, string, string) error { return nil }}
		w := serveAgent(t, svc, cleaner, del, agentCall{method: http.MethodDelete, url: "/x"})
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, []string{callerOU + "/proj/agent"}, cleaner.agents)
	})

	t.Run("a failed delete leaves the subscriptions", func(t *testing.T) {
		cleaner := &recordingCleaner{}
		svc := &stubAgentService{del: func(string, string, string) error { return utils.ErrAgentNotFound }}
		w := serveAgent(t, svc, cleaner, del, agentCall{method: http.MethodDelete, url: "/x"})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, cleaner.agents)
	})

	t.Run("no cleaner configured", func(t *testing.T) {
		svc := &stubAgentService{del: func(string, string, string) error { return nil }}
		assert.Equal(t, http.StatusNoContent, serveAgent(t, svc, nil, del, agentCall{method: http.MethodDelete, url: "/x"}).Code)
	})
}

func TestAgentController_GenerateName(t *testing.T) {
	gen := func(c AgentController) http.HandlerFunc { return c.GenerateName }

	w := serveAgent(t, &stubAgentService{}, nil, gen, agentCall{method: http.MethodPost, url: "/x", body: "{"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAgent(t, &stubAgentService{}, nil, gen, agentCall{method: http.MethodPost, url: "/x", body: `{"displayName":"x","resourceType":"widget"}`})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "Invalid resource name payload", decodeError(t, w).Message)

	svc := &stubAgentService{genName: func(ouID string, req spec.ResourceNameRequest) (string, error) {
		assert.Equal(t, callerOU, ouID)
		return "my-agent-x1y2", nil
	}}
	w = serveAgent(t, svc, nil, gen, agentCall{method: http.MethodPost, url: "/x", body: `{"displayName":"My Agent","resourceType":"agent","projectName":"proj"}`})
	require.Equal(t, http.StatusOK, w.Code)
	var resp spec.ResourceNameResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, spec.ResourceNameResponse{Name: "my-agent-x1y2", DisplayName: "My Agent", ResourceType: "agent"}, resp)

	svc.genName = func(string, spec.ResourceNameRequest) (string, error) { return "", utils.ErrProjectNotFound }
	w = serveAgent(t, svc, nil, gen, agentCall{method: http.MethodPost, url: "/x", body: `{"displayName":"My Agent","resourceType":"agent"}`})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAgentController_GetAgentIdentity(t *testing.T) {
	get := func(c AgentController) http.HandlerFunc { return c.GetAgentIdentity }
	views := []models.AgentIdentityEnvironmentView{
		{EnvironmentName: "dev", ClientID: "c-dev"},
		{EnvironmentName: "prod", ClientID: "c-prod"},
	}
	svc := &stubAgentService{identity: func() ([]models.AgentIdentityEnvironmentView, error) { return views, nil }}

	decode := func(w *httptest.ResponseRecorder) []models.AgentIdentityEnvironmentView {
		require.Equal(t, http.StatusOK, w.Code)
		var got []models.AgentIdentityEnvironmentView
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}
	assert.Len(t, decode(serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x"})), 2)
	filtered := decode(serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x?environment=prod"}))
	require.Len(t, filtered, 1)
	assert.Equal(t, "c-prod", filtered[0].ClientID)

	// The filter and an agent with no bindings both still answer an array, never null.
	w := serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x?environment=qa"})
	assert.JSONEq(t, "[]", w.Body.String())
	svc.identity = func() ([]models.AgentIdentityEnvironmentView, error) { return nil, nil }
	w = serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x"})
	assert.JSONEq(t, "[]", w.Body.String())

	svc.identity = func() ([]models.AgentIdentityEnvironmentView, error) { return nil, utils.ErrAgentNotFound }
	assert.Equal(t, http.StatusNotFound, serveAgent(t, svc, nil, get, agentCall{method: http.MethodGet, url: "/x"}).Code)
}

func TestAgentController_RegenerateAgentIdentitySecret(t *testing.T) {
	regen := func(c AgentController) http.HandlerFunc { return c.RegenerateAgentIdentitySecret }
	post := func(body string, rec audit.Recorder) agentCall {
		return agentCall{method: http.MethodPost, url: "/x", body: body, rec: rec}
	}

	assert.Equal(t, http.StatusBadRequest, serveAgent(t, &stubAgentService{}, nil, regen, post("{", nil)).Code)
	defaulted := &stubAgentService{regenerate: func(env string) (*models.AgentRegenerateSecretResponse, error) {
		assert.Equal(t, models.DefaultEnvironmentName, env, "an absent environment is the default one")
		return &models.AgentRegenerateSecretResponse{EnvironmentName: env}, nil
	}}
	assert.Equal(t, http.StatusOK, serveAgent(t, defaulted, nil, regen, post(`{}`, &captureRecorder{})).Code)
	assert.Equal(t, http.StatusServiceUnavailable,
		serveAgent(t, &stubAgentService{}, nil, regen, post(`{"environment":"dev"}`, &captureRecorder{failSync: true})).Code,
		"a secret is never rotated unrecorded")

	rec := &captureRecorder{}
	svc := &stubAgentService{regenerate: func(env string) (*models.AgentRegenerateSecretResponse, error) {
		assert.Equal(t, "dev", env)
		return &models.AgentRegenerateSecretResponse{EnvironmentName: env, ClientID: "cid", ClientSecret: "fresh-secret"}, nil
	}}
	w := serveAgent(t, svc, nil, regen, post(`{"environment":"dev"}`, rec))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "fresh-secret", decodeMap(t, w)["clientSecret"])
	done := rec.completed(audit.ActionAgentIdentityRegenerateSecret)
	require.Len(t, done, 1)
	assert.Equal(t, audit.OutcomeSuccess, done[0].Outcome)
	raw, _ := json.Marshal(rec.events)
	assert.NotContains(t, string(raw), "fresh-secret")

	svc.regenerate = func(string) (*models.AgentRegenerateSecretResponse, error) {
		return nil, utils.ErrAgentIdentityNotProvisioned
	}
	w = serveAgent(t, svc, nil, regen, post(`{"environment":"dev"}`, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "Agent identity not yet provisioned for this environment", decodeError(t, w).Message)

	rec = &captureRecorder{}
	svc.regenerate = func(string) (*models.AgentRegenerateSecretResponse, error) { return nil, utils.ErrAgentNotFound }
	w = serveAgent(t, svc, nil, regen, post(`{"environment":"dev"}`, rec))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, utils.ErrCodeAgentNotFound, decodeError(t, w).Code)
	assert.Equal(t, audit.OutcomeFailure, rec.completed(audit.ActionAgentIdentityRegenerateSecret)[0].Outcome)
}

func TestAgentController_RevokeAgentIdentitySecret(t *testing.T) {
	revoke := func(c AgentController) http.HandlerFunc { return c.RevokeAgentIdentitySecret }

	defaulted := &stubAgentService{revoke: func(env string) (models.AgentRevokeSecretResponse, error) {
		assert.Equal(t, models.DefaultEnvironmentName, env, "an absent environment is the default one")
		return models.AgentRevokeSecretResponse{EnvironmentName: env, Status: "revoked"}, nil
	}}
	assert.Equal(t, http.StatusOK, serveAgent(t, defaulted, nil, revoke,
		agentCall{method: http.MethodDelete, url: "/x", rec: &captureRecorder{}}).Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAgent(t, &stubAgentService{}, nil, revoke,
		agentCall{method: http.MethodDelete, url: "/x?environment=dev", rec: &captureRecorder{failSync: true}}).Code)

	svc := &stubAgentService{revoke: func(env string) (models.AgentRevokeSecretResponse, error) {
		return models.AgentRevokeSecretResponse{EnvironmentName: env, Status: "revoked"}, nil
	}}
	w := serveAgent(t, svc, nil, revoke, agentCall{method: http.MethodDelete, url: "/x?environment=dev"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "revoked", decodeMap(t, w)["status"])

	for err, status := range map[error]int{
		utils.ErrAgentIdentityNotProvisioned: http.StatusNotFound,
		errThunder:                           http.StatusInternalServerError,
	} {
		svc.revoke = func(string) (models.AgentRevokeSecretResponse, error) { return models.AgentRevokeSecretResponse{}, err }
		assert.Equal(t, status, serveAgent(t, svc, nil, revoke, agentCall{method: http.MethodDelete, url: "/x?environment=dev"}).Code)
	}
}

func TestAgentController_ProvisionAgentIdentity(t *testing.T) {
	provision := func(c AgentController) http.HandlerFunc { return c.ProvisionAgentIdentity }

	defaulted := &stubAgentService{provision: func(env string) (models.AgentIdentityEnvironmentView, bool, error) {
		assert.Equal(t, models.DefaultEnvironmentName, env, "an absent environment is the default one")
		return models.AgentIdentityEnvironmentView{EnvironmentName: env}, false, nil
	}}
	assert.Equal(t, http.StatusAccepted, serveAgent(t, defaulted, nil, provision,
		agentCall{method: http.MethodPut, url: "/x", rec: &captureRecorder{}}).Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAgent(t, &stubAgentService{}, nil, provision,
		agentCall{method: http.MethodPut, url: "/x?environment=dev", rec: &captureRecorder{failSync: true}}).Code)

	t.Run("a new binding is accepted", func(t *testing.T) {
		rec := &captureRecorder{}
		svc := &stubAgentService{provision: func(env string) (models.AgentIdentityEnvironmentView, bool, error) {
			return models.AgentIdentityEnvironmentView{EnvironmentName: env, Status: "pending"}, false, nil
		}}
		w := serveAgent(t, svc, nil, provision, agentCall{method: http.MethodPut, url: "/x?environment=dev", rec: rec})
		assert.Equal(t, http.StatusAccepted, w.Code)
		done := rec.completed(audit.ActionAgentIdentityProvision)
		require.Len(t, done, 1)
		assert.Nil(t, done[0].Details["alreadyExisted"])
	})

	t.Run("an existing binding is a no-op 200 recorded as a notice", func(t *testing.T) {
		rec := &captureRecorder{}
		svc := &stubAgentService{provision: func(env string) (models.AgentIdentityEnvironmentView, bool, error) {
			return models.AgentIdentityEnvironmentView{EnvironmentName: env, Status: "active"}, true, nil
		}}
		w := serveAgent(t, svc, nil, provision, agentCall{method: http.MethodPut, url: "/x?environment=dev", rec: rec})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "dev", decodeMap(t, w)["environmentName"])
		done := rec.completed(audit.ActionAgentIdentityProvision)
		require.Len(t, done, 1)
		assert.Equal(t, audit.SeverityNotice, done[0].Severity)
		assert.Equal(t, true, done[0].Details["alreadyExisted"])
	})

	t.Run("failure", func(t *testing.T) {
		svc := &stubAgentService{provision: func(string) (models.AgentIdentityEnvironmentView, bool, error) {
			return models.AgentIdentityEnvironmentView{}, false, utils.ErrEnvironmentNotFound
		}}
		w := serveAgent(t, svc, nil, provision, agentCall{method: http.MethodPut, url: "/x?environment=dev"})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, utils.ErrCodeEnvironmentNotFound, decodeError(t, w).Code)
	})
}

func TestAgentController_RetryAgentIdentityProvisioning(t *testing.T) {
	retry := func(c AgentController) http.HandlerFunc { return c.RetryAgentIdentityProvisioning }
	post := func(body string) agentCall { return agentCall{method: http.MethodPost, url: "/x", body: body} }

	assert.Equal(t, http.StatusBadRequest, serveAgent(t, &stubAgentService{}, nil, retry, post("{")).Code)
	defaulted := &stubAgentService{retry: func(env string) (models.AgentIdentityEnvironmentView, error) {
		assert.Equal(t, models.DefaultEnvironmentName, env, "an absent environment is the default one")
		return models.AgentIdentityEnvironmentView{EnvironmentName: env}, nil
	}}
	assert.Equal(t, http.StatusAccepted, serveAgent(t, defaulted, nil, retry,
		agentCall{method: http.MethodPost, url: "/x", body: `{"environment":""}`, rec: &captureRecorder{}}).Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAgent(t, &stubAgentService{}, nil, retry,
		agentCall{method: http.MethodPost, url: "/x", body: `{"environment":"dev"}`, rec: &captureRecorder{failSync: true}}).Code)

	svc := &stubAgentService{retry: func(env string) (models.AgentIdentityEnvironmentView, error) {
		return models.AgentIdentityEnvironmentView{EnvironmentName: env, Status: "pending"}, nil
	}}
	w := serveAgent(t, svc, nil, retry, post(`{"environment":"dev"}`))
	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "pending", decodeMap(t, w)["status"])

	for err, want := range map[error]struct {
		status int
		msg    string
	}{
		utils.ErrAgentIdentityNotProvisioned:  {http.StatusNotFound, "Agent identity not yet provisioned for this environment"},
		utils.ErrAgentIdentityRetryNotAllowed: {http.StatusConflict, "Agent identity binding is not in a failed state and cannot be retried"},
		errThunder:                            {http.StatusInternalServerError, "Failed to retry agent identity provisioning"},
	} {
		svc.retry = func(string) (models.AgentIdentityEnvironmentView, error) {
			return models.AgentIdentityEnvironmentView{}, err
		}
		w := serveAgent(t, svc, nil, retry, post(`{"environment":"dev"}`))
		assert.Equal(t, want.status, w.Code)
		assert.Equal(t, want.msg, decodeError(t, w).Message)
	}
}

func TestAgentController_RolesAndGroups(t *testing.T) {
	type op struct {
		h   func(AgentController) http.HandlerFunc
		key string
		set func(*stubAgentService, error)
	}
	ops := map[string]op{
		"roles": {
			func(c AgentController) http.HandlerFunc { return c.GetAgentRoles }, "roles",
			func(s *stubAgentService, err error) {
				s.roles = func(env string) ([]thundersvc.ThunderRole, error) {
					assert.Equal(t, "dev", env)
					return []thundersvc.ThunderRole{{ID: "r1"}}, err
				}
			},
		},
		"groups": {
			func(c AgentController) http.HandlerFunc { return c.GetAgentGroups }, "groups",
			func(s *stubAgentService, err error) {
				s.groups = func(string) ([]thundersvc.ThunderGroup, error) { return []thundersvc.ThunderGroup{{ID: "g1"}}, err }
			},
		},
	}
	for name, o := range ops {
		t.Run(name, func(t *testing.T) {
			defaulted := &stubAgentService{
				roles: func(env string) ([]thundersvc.ThunderRole, error) {
					assert.Equal(t, models.DefaultEnvironmentName, env)
					return nil, nil
				},
				groups: func(env string) ([]thundersvc.ThunderGroup, error) {
					assert.Equal(t, models.DefaultEnvironmentName, env)
					return nil, nil
				},
			}
			w := serveAgent(t, defaulted, nil, o.h, agentCall{method: http.MethodGet, url: "/x"})
			assert.Equal(t, http.StatusOK, w.Code, "an absent environment is the default one")

			svc := &stubAgentService{}
			o.set(svc, nil)
			w = serveAgent(t, svc, nil, o.h, agentCall{method: http.MethodGet, url: "/x?environment=dev"})
			require.Equal(t, http.StatusOK, w.Code)
			assert.Len(t, decodeMap(t, w)[o.key], 1)

			for err, want := range map[error]struct {
				status int
				msg    string
			}{
				utils.ErrAgentIdentityNotProvisioned: {http.StatusNotFound, "Agent identity not yet provisioned for this environment"},
				thundersvc.ErrThunderNotProvisioned:  {http.StatusNotFound, "The identity provider is not set up for this environment yet"},
				utils.ErrAgentNotFound:               {http.StatusNotFound, "Agent not found"},
			} {
				o.set(svc, fmt.Errorf("wrapped: %w", err))
				w = serveAgent(t, svc, nil, o.h, agentCall{method: http.MethodGet, url: "/x?environment=dev"})
				assert.Equal(t, want.status, w.Code)
				assert.Equal(t, want.msg, decodeError(t, w).Message)
			}
		})
	}
}
