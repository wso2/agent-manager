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
package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// Each stub embeds the service interface so an unexpected call panics on the nil embedded value.

type agentSvcStub struct {
	services.AgentManagerService
	listFn   func(ouID, proj string, labels map[string]string, limit, offset int32) ([]*models.AgentResponse, int32, error)
	getFn    func(ouID, proj, agent string) (*models.AgentResponse, error)
	createFn func(ouID, proj string, req *spec.CreateAgentRequest) error
}

func (s *agentSvcStub) ListAgents(_ context.Context, ouID, proj string, labels map[string]string, limit, offset int32) ([]*models.AgentResponse, int32, error) {
	return s.listFn(ouID, proj, labels, limit, offset)
}

func (s *agentSvcStub) GetAgent(_ context.Context, ouID, proj, agent string) (*models.AgentResponse, error) {
	return s.getFn(ouID, proj, agent)
}

func (s *agentSvcStub) CreateAgent(_ context.Context, ouID, proj string, req *spec.CreateAgentRequest) error {
	return s.createFn(ouID, proj, req)
}

type tokenSvcStub struct {
	services.AgentTokenManagerService
	got services.GenerateTokenRequest
	err error
}

func (s *tokenSvcStub) GenerateToken(_ context.Context, req services.GenerateTokenRequest) (*spec.TokenResponse, error) {
	s.got = req
	if s.err != nil {
		return nil, s.err
	}
	return &spec.TokenResponse{Token: "signed"}, nil
}

func TestAgentHandlerForwardsToService(t *testing.T) {
	errBoom := errors.New("boom")
	svc := &agentSvcStub{
		listFn: func(ouID, proj string, labels map[string]string, limit, offset int32) ([]*models.AgentResponse, int32, error) {
			assert.Equal(t, "org", ouID)
			assert.Equal(t, "proj", proj)
			assert.Nil(t, labels, "MCP lists agents without a label filter")
			assert.Equal(t, int32(5), limit)
			assert.Equal(t, int32(10), offset)
			return []*models.AgentResponse{{Name: "a1"}}, 11, nil
		},
		getFn: func(ouID, proj, agent string) (*models.AgentResponse, error) {
			if agent == "missing" {
				return nil, errBoom
			}
			return &models.AgentResponse{Name: agent}, nil
		},
		createFn: func(ouID, proj string, req *spec.CreateAgentRequest) error {
			assert.Equal(t, "new", req.Name)
			return errBoom
		},
	}
	h := NewAgentHandler(svc, nil)
	ctx := context.Background()

	agents, total, err := h.ListAgents(ctx, "org", "proj", 5, 10)
	require.NoError(t, err)
	assert.Equal(t, int32(11), total)
	require.Len(t, agents, 1)
	assert.Equal(t, "a1", agents[0].Name)

	got, err := h.GetAgent(ctx, "org", "proj", "a1")
	require.NoError(t, err)
	assert.Equal(t, "a1", got.Name)

	_, err = h.GetAgent(ctx, "org", "proj", "missing")
	require.ErrorIs(t, err, errBoom)

	require.ErrorIs(t, h.CreateAgent(ctx, "org", "proj", &spec.CreateAgentRequest{Name: "new"}), errBoom)
}

func TestAgentHandlerGenerateToken(t *testing.T) {
	withClaims := jwtassertion.ContextWithTokenClaims(context.Background(), &jwtassertion.TokenClaims{OuId: "ou-uuid"})

	t.Run("no token service", func(t *testing.T) {
		_, err := NewAgentHandler(&agentSvcStub{}, nil).GenerateToken(withClaims, "org", "p", "a", "dev", "1h")
		require.ErrorContains(t, err, "token service not configured")
	})

	t.Run("caller without organization", func(t *testing.T) {
		tok := &tokenSvcStub{}
		h := NewAgentHandler(&agentSvcStub{}, tok)
		for name, ctx := range map[string]context.Context{
			"no claims":   context.Background(),
			"empty ou id": jwtassertion.ContextWithTokenClaims(context.Background(), &jwtassertion.TokenClaims{Sub: "u"}),
		} {
			_, err := h.GenerateToken(ctx, "org", "p", "a", "dev", "1h")
			require.ErrorContains(t, err, "organization identity missing", name)
		}
		assert.Empty(t, tok.got.OrgName, "token service must not be called")
	})

	t.Run("builds the request from args and caller", func(t *testing.T) {
		tok := &tokenSvcStub{}
		resp, err := NewAgentHandler(&agentSvcStub{}, tok).GenerateToken(withClaims, "org", "p", "a", "dev", "1h")
		require.NoError(t, err)
		assert.Equal(t, "signed", resp.Token)
		assert.Equal(t, services.GenerateTokenRequest{
			OrgName: "org", ProjectName: "p", AgentName: "a", Environment: "dev", ExpiresIn: "1h", OrgId: "ou-uuid",
		}, tok.got)
	})

	t.Run("service error is returned", func(t *testing.T) {
		errDenied := errors.New("denied")
		_, err := NewAgentHandler(&agentSvcStub{}, &tokenSvcStub{err: errDenied}).GenerateToken(withClaims, "org", "p", "a", "", "")
		require.ErrorIs(t, err, errDenied)
	})
}

type envSvcStub struct {
	services.EnvironmentService
	gotOU, gotName      string
	gotLimit, gotOffset int32
	err                 error
}

func (s *envSvcStub) ListEnvironments(_ context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error) {
	s.gotOU, s.gotLimit, s.gotOffset = ouID, limit, offset
	return &models.EnvironmentListResponse{Total: 1, Environments: []models.GatewayEnvironmentResponse{{Name: "dev"}}}, s.err
}

func (s *envSvcStub) GetEnvironment(_ context.Context, ouID, envName string) (*models.GatewayEnvironmentResponse, error) {
	s.gotOU, s.gotName = ouID, envName
	if s.err != nil {
		return nil, s.err
	}
	return &models.GatewayEnvironmentResponse{Name: envName}, nil
}

func TestEnvironmentHandler(t *testing.T) {
	svc := &envSvcStub{}
	h := NewEnvironmentHandler(svc)

	list, err := h.ListEnvironments(context.Background(), "org", 20, 40)
	require.NoError(t, err)
	assert.Equal(t, int32(1), list.Total)
	assert.Equal(t, "org", svc.gotOU)
	assert.Equal(t, int32(20), svc.gotLimit)
	assert.Equal(t, int32(40), svc.gotOffset)

	env, err := h.GetEnvironment(context.Background(), "org", "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", env.Name)
	assert.Equal(t, "dev", svc.gotName)

	svc.err = errors.New("not found")
	_, err = h.GetEnvironment(context.Background(), "org", "prod")
	require.ErrorIs(t, err, svc.err)
}

type infraSvcStub struct {
	services.InfraResourceManager
	gotLimit, gotOffset int
	gotPayload          spec.CreateProjectRequest
	err                 error
}

func (s *infraSvcStub) ListProjects(_ context.Context, ouID string, limit, offset int) ([]*models.ProjectResponse, int32, error) {
	s.gotLimit, s.gotOffset = limit, offset
	return []*models.ProjectResponse{{Name: "p1", OrgName: ouID}}, 1, s.err
}

func (s *infraSvcStub) CreateProject(_ context.Context, ouID string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.gotPayload = payload
	if s.err != nil {
		return nil, s.err
	}
	return &models.ProjectResponse{Name: payload.Name, OrgName: ouID}, nil
}

func TestProjectHandler(t *testing.T) {
	svc := &infraSvcStub{}
	h := NewProjectHandler(svc)

	projects, total, err := h.ListProjects(context.Background(), "org", 3, 6)
	require.NoError(t, err)
	assert.Equal(t, int32(1), total)
	assert.Equal(t, "org", projects[0].OrgName)
	assert.Equal(t, 3, svc.gotLimit)
	assert.Equal(t, 6, svc.gotOffset)

	p, err := h.CreateProject(context.Background(), "org", spec.CreateProjectRequest{Name: "new"})
	require.NoError(t, err)
	assert.Equal(t, "new", p.Name)
	assert.Equal(t, "new", svc.gotPayload.Name)

	svc.err = errors.New("conflict")
	_, err = h.CreateProject(context.Background(), "org", spec.CreateProjectRequest{Name: "dup"})
	require.ErrorIs(t, err, svc.err)
}
