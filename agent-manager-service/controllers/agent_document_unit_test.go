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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// failingReader fails every read, standing in for a client that drops mid-body.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestAgentController_ProjectScopedHandlersRequireProject(t *testing.T) {
	handlers := map[string]func(AgentController) http.HandlerFunc{
		"update":    func(c AgentController) http.HandlerFunc { return c.UpdateAgentBasicInfo },
		"delete":    func(c AgentController) http.HandlerFunc { return c.DeleteAgent },
		"identity":  func(c AgentController) http.HandlerFunc { return c.GetAgentIdentity },
		"regen":     func(c AgentController) http.HandlerFunc { return c.RegenerateAgentIdentitySecret },
		"revoke":    func(c AgentController) http.HandlerFunc { return c.RevokeAgentIdentitySecret },
		"provision": func(c AgentController) http.HandlerFunc { return c.ProvisionAgentIdentity },
		"retry":     func(c AgentController) http.HandlerFunc { return c.RetryAgentIdentityProvisioning },
		"roles":     func(c AgentController) http.HandlerFunc { return c.GetAgentRoles },
		"groups":    func(c AgentController) http.HandlerFunc { return c.GetAgentGroups },
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			w := serveAgent(t, &stubAgentService{}, nil, h, agentCall{method: http.MethodPost, url: "/x", body: "{}", noProject: true})
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestAgentController_UnreadableBody(t *testing.T) {
	handlers := map[string]func(AgentController) http.HandlerFunc{
		"create": func(c AgentController) http.HandlerFunc { return c.CreateAgent },
		"update": func(c AgentController) http.HandlerFunc { return c.UpdateAgentBasicInfo },
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/x?project=proj", failingReader{})
			req.SetPathValue(utils.PathParamAgentName, "agent")
			ctx := middleware.WithResolvedOrg(req.Context(), orgctx.ResolvedOrg{OUID: callerOU, OuHandle: "acme"})
			ctx = audit.WithRecorder(ctx, audit.NewNoopRecorder())
			w := httptest.NewRecorder()
			h(NewAgentController(&stubAgentService{}, nil))(w, req.WithContext(ctx))
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "Invalid request body", decodeError(t, w).Message)
		})
	}
}

func TestAgentDocument_OptionalFields(t *testing.T) {
	a := agentModel("a")
	a.CreatedBy = &models.AgentCreatedBy{ID: "user-1"}
	a.Type.SubType = "chat-api"
	a.Status = "active"
	doc := agentDocument(a)
	require.NotNil(t, doc.Metadata.CreatedBy)
	assert.Equal(t, "user-1", *doc.Metadata.CreatedBy)
	require.NotNil(t, doc.Spec.AgentType.SubType)
	assert.Equal(t, "chat-api", *doc.Spec.AgentType.SubType)
	require.NotNil(t, doc.Status)
	assert.Equal(t, "active", *doc.Status.State)

	bare := agentDocument(&models.AgentResponse{Name: "b", CreatedBy: &models.AgentCreatedBy{}, CreatedAt: time.Time{}})
	assert.Nil(t, bare.Metadata.Uid)
	assert.Nil(t, bare.Metadata.CreationTimestamp)
	assert.Nil(t, bare.Metadata.CreatedBy)
	assert.Nil(t, bare.Status)
}

func TestDecodeAgentDocument_Errors(t *testing.T) {
	cases := map[string]struct {
		body      string
		malformed bool
		message   string
	}{
		"wrong field type": {body: `{"metadata":"x"}`, message: "cannot unmarshal"},
		"no name": {
			body:    `{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"Agent","metadata":{"name":" ","project":"proj"},"spec":{}}`,
			message: "metadata.name is required",
		},
		"no project": {
			body:    `{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"Agent","metadata":{"name":"a"},"spec":{}}`,
			message: "metadata.project is required",
		},
		"no spec": {
			body:    `{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"Agent","metadata":{"name":"a","project":"proj"}}`,
			message: "spec is required",
		},
		"not json": {body: `{`, malformed: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := decodeAgentDocument([]byte(tc.body))
			require.Error(t, err)
			assert.Equal(t, tc.malformed, errors.Is(err, errMalformedBody))
			if !tc.malformed {
				assert.ErrorIs(t, err, errDocument)
				assert.Contains(t, err.Error(), tc.message)
			}
		})
	}
}
