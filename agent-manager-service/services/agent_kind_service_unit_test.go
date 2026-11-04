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

// This file is the REFERENCE for service-layer UNIT tests.
//
// Unlike the rest of services/*_test.go (which carry `//go:build integration`
// and run against a real Postgres), this file has NO build tag, so it runs in
// the fast unit tier (`make test-unit`). It exercises agentKindService's
// business logic with the dependencies mocked:
//
//   - repositories.AgentKindRepository -> repomocks.AgentKindRepositoryMock
//     (moq-generated; see //go:generate directive in the interface file and
//     `make codegen`)
//   - occlient.OpenChoreoClient        -> clientmocks.OpenChoreoClientMock
//     (moq-generated in clients/clientmocks)
//
// The goal is to assert the service's OWN logic — error mapping, validation
// gates, branching, fan-out — without touching the database. Pattern to copy
// for other services: inject the generated mock for every interface dependency,
// drive each branch, and assert on the returned value/error (errors.Is for
// sentinels). An unconfigured mock method panics, so a test that hits an
// unexpected code path fails loudly rather than silently returning a zero value.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// newKindService wires the service with the two mocked dependencies.
func newKindService(repo *repomocks.AgentKindRepositoryMock, oc *clientmocks.OpenChoreoClientMock) AgentKindService {
	return NewAgentKindService(repo, oc)
}

// Note: strPtr (helper for *string fields on generated spec structs) is already
// declared in llm_deployment_service_test.go in this package, so we reuse it.

// buildWithImage builds a BuildDetailsResponse whose ImageId (on the embedded
// BuildResponse) is set — the field can't be assigned in a flat literal.
func buildWithImage(imageID string) *models.BuildDetailsResponse {
	return &models.BuildDetailsResponse{
		BuildResponse: models.BuildResponse{ImageId: imageID},
	}
}

// -----------------------------------------------------------------------------
// GetKind — demonstrates error-mapping branches (the most common service logic).
// -----------------------------------------------------------------------------

func TestAgentKindService_GetKind(t *testing.T) {
	const org, kindName = "acme", "chatbot"
	kindID := uuid.New()

	t.Run("maps record-not-found to ErrAgentKindNotFound", func(t *testing.T) {
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		_, err := svc.GetKind(context.Background(), org, kindName)

		// Sentinel must be compared with errors.Is, not == or string match.
		assert.ErrorIs(t, err, utils.ErrAgentKindNotFound)
	})

	t.Run("wraps an unexpected repo error (does NOT mask as not-found)", func(t *testing.T) {
		boom := errors.New("connection reset")
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return nil, boom
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		_, err := svc.GetKind(context.Background(), org, kindName)

		require.Error(t, err)
		// Real errors must surface, NOT be flattened into ErrAgentKindNotFound.
		assert.NotErrorIs(t, err, utils.ErrAgentKindNotFound)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("returns mapped response on success", func(t *testing.T) {
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return &models.AgentKind{
					ID:          kindID,
					Name:        kindName,
					DisplayName: "Chatbot",
					OUID:        org,
					Versions: []models.AgentKindVersion{
						{Version: "v2"}, // first entry => latest
						{Version: "v1"},
					},
				}, nil
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		resp, err := svc.GetKind(context.Background(), org, kindName)

		require.NoError(t, err)
		assert.Equal(t, kindName, resp.Name)
		assert.Equal(t, "v2", resp.LatestVersion)
		assert.Len(t, resp.Versions, 2)
	})

	t.Run("redacts secret config defaults without mutating the source model", func(t *testing.T) {
		apiKey := "sk-real-secret-value"
		modelName := "gpt-4"
		source := &models.AgentKind{
			ID:          kindID,
			Name:        kindName,
			DisplayName: "Chatbot",
			OUID:        org,
			Versions: []models.AgentKindVersion{
				{
					Version: "v1",
					ConfigSchema: []models.KindConfigSchemaItem{
						{Name: "OPENAI_API_KEY", IsSecret: true, IsMandatory: true, DefaultValue: &apiKey},
						{Name: "MODEL_NAME", IsSecret: false, DefaultValue: &modelName},
					},
				},
			},
		}
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return source, nil
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		resp, err := svc.GetKind(context.Background(), org, kindName)
		require.NoError(t, err)
		require.Len(t, resp.Versions, 1)
		schema := resp.Versions[0].ConfigSchema
		require.Len(t, schema, 2)
		if assert.NotNil(t, schema[0].DefaultValue, "a secret item WITH a default must still signal one exists") {
			assert.Equal(t, models.RedactedSecretDefaultPlaceholder, *schema[0].DefaultValue, "the real secret value must never reach the client")
		}
		if assert.NotNil(t, schema[1].DefaultValue) {
			assert.Equal(t, modelName, *schema[1].DefaultValue, "non-secret default is unaffected")
		}

		// The GORM-loaded source model must not have been mutated by building the response.
		require.NotNil(t, source.Versions[0].ConfigSchema[0].DefaultValue)
		assert.Equal(t, apiKey, *source.Versions[0].ConfigSchema[0].DefaultValue)
	})
}

// -----------------------------------------------------------------------------
// DeleteKind — demonstrates a guard that depends on BOTH a repo call and a
// client fan-out (ListKindAgents). Shows mocking two collaborators at once.
// -----------------------------------------------------------------------------

func TestAgentKindService_DeleteKind(t *testing.T) {
	const org, kindName = "acme", "chatbot"

	t.Run("refuses deletion when instances exist", func(t *testing.T) {
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return &models.AgentKind{Name: kindName}, nil
			},
			// DeleteKind must NOT be reached — leaving it nil asserts that.
		}
		oc := &clientmocks.OpenChoreoClientMock{
			ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
				return []*models.ProjectResponse{{Name: "proj-a"}}, nil
			},
			ListComponentsByKindFunc: func(_ context.Context, _, _, _ string) ([]*models.AgentResponse, error) {
				return []*models.AgentResponse{{Name: "running-agent"}}, nil
			},
		}
		svc := newKindService(repo, oc)

		err := svc.DeleteKind(context.Background(), org, kindName)

		assert.ErrorIs(t, err, utils.ErrAgentKindHasInstances)
	})

	t.Run("deletes when no instances exist", func(t *testing.T) {
		deleteCalled := false
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return &models.AgentKind{Name: kindName}, nil
			},
			DeleteKindFunc: func(_ context.Context, _, _ string) error {
				deleteCalled = true
				return nil
			},
		}
		oc := &clientmocks.OpenChoreoClientMock{
			ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
				return []*models.ProjectResponse{{Name: "proj-a"}}, nil
			},
			ListComponentsByKindFunc: func(_ context.Context, _, _, _ string) ([]*models.AgentResponse, error) {
				return []*models.AgentResponse{}, nil // no agents from this kind
			},
		}
		svc := newKindService(repo, oc)

		err := svc.DeleteKind(context.Background(), org, kindName)

		require.NoError(t, err)
		assert.True(t, deleteCalled, "expected repo.DeleteKind to be called")
	})
}

// -----------------------------------------------------------------------------
// AddVersion -> publishVersion — demonstrates driving several validation gates,
// each surfaced as a distinct sentinel error. This is the highest-value kind of
// service unit test: pure branch coverage that a real DB would make slow/awkward.
// -----------------------------------------------------------------------------

func TestAgentKindService_AddVersion_Gates(t *testing.T) {
	const org, kindName = "acme", "chatbot"
	kindID := uuid.New()

	baseRepo := func() *repomocks.AgentKindRepositoryMock {
		return &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return &models.AgentKind{ID: kindID, Name: kindName, OUID: org}, nil
			},
		}
	}
	req := &spec.AddAgentKindVersionRequest{}

	t.Run("rejects when version already exists", func(t *testing.T) {
		repo := baseRepo()
		repo.GetVersionFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.AgentKindVersion, error) {
			return &models.AgentKindVersion{Version: "v1"}, nil // already present
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		_, err := svc.AddVersion(context.Background(), org, kindName, req)

		assert.ErrorIs(t, err, utils.ErrKindVersionAlreadyExists)
	})

	t.Run("rejects when build image is not ready", func(t *testing.T) {
		repo := baseRepo()
		repo.GetVersionFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.AgentKindVersion, error) {
			return nil, gorm.ErrRecordNotFound // version slot is free
		}
		oc := &clientmocks.OpenChoreoClientMock{
			GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
				return &models.AgentResponse{Name: "src-agent"}, nil // source component exists
			},
			GetBuildFunc: func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
				return buildWithImage(""), nil // build incomplete
			},
		}
		svc := newKindService(repo, oc)

		_, err := svc.AddVersion(context.Background(), org, kindName, req)

		assert.ErrorIs(t, err, utils.ErrBuildNotComplete)
	})

	t.Run("reports build not found when the build belongs to another agent", func(t *testing.T) {
		repo := baseRepo()
		repo.GetVersionFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.AgentKindVersion, error) {
			return nil, gorm.ErrRecordNotFound
		}
		oc := &clientmocks.OpenChoreoClientMock{
			GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
				return &models.AgentResponse{Name: "src-agent"}, nil
			},
			GetBuildFunc: func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
				return nil, utils.ErrNotFound // scoped lookup rejects a foreign build
			},
		}
		svc := newKindService(repo, oc)

		_, err := svc.AddVersion(context.Background(), org, kindName, req)

		assert.ErrorIs(t, err, utils.ErrBuildNotFound)
	})

	t.Run("rejects when image already published under this kind", func(t *testing.T) {
		repo := baseRepo()
		repo.GetVersionFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.AgentKindVersion, error) {
			return nil, gorm.ErrRecordNotFound
		}
		repo.GetVersionByImageIDFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.AgentKindVersion, error) {
			return &models.AgentKindVersion{Version: "v1"}, nil // dup image
		}
		oc := &clientmocks.OpenChoreoClientMock{
			GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
				return &models.AgentResponse{Name: "src-agent"}, nil
			},
			GetBuildFunc: func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
				return buildWithImage("sha256:abc"), nil
			},
		}
		svc := newKindService(repo, oc)

		_, err := svc.AddVersion(context.Background(), org, kindName, req)

		assert.ErrorIs(t, err, utils.ErrKindImageAlreadyPublished)
	})

	t.Run("rejects a secret default equal to the redaction placeholder", func(t *testing.T) {
		repo := baseRepo()
		item := spec.AgentKindConfigSchemaItem{Name: "OPENAI_API_KEY", IsSecret: true, IsMandatory: true}
		item.SetDefaultValue(models.RedactedSecretDefaultPlaceholder)
		placeholderReq := &spec.AddAgentKindVersionRequest{ConfigSchema: []spec.AgentKindConfigSchemaItem{item}}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		_, err := svc.AddVersion(context.Background(), org, kindName, placeholderReq)

		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Empty(t, repo.CreateVersionCalls(), "must be rejected before ever reaching persistence")
	})

	t.Run("persists a new version on the happy path", func(t *testing.T) {
		var created *models.AgentKindVersion
		repo := baseRepo()
		repo.GetVersionFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.AgentKindVersion, error) {
			return nil, gorm.ErrRecordNotFound
		}
		repo.GetVersionByImageIDFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.AgentKindVersion, error) {
			return nil, gorm.ErrRecordNotFound
		}
		repo.FindVersionByImageIDInOrgFunc = func(_ context.Context, _, _ string) (*models.AgentKindVersion, error) {
			return nil, gorm.ErrRecordNotFound
		}
		repo.CreateVersionFunc = func(_ context.Context, v *models.AgentKindVersion) error {
			created = v
			return nil
		}
		oc := &clientmocks.OpenChoreoClientMock{
			GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
				return &models.AgentResponse{Name: "src-agent"}, nil
			},
			GetBuildFunc: func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
				return buildWithImage("sha256:abc"), nil
			},
		}
		svc := newKindService(repo, oc)

		_, err := svc.AddVersion(context.Background(), org, kindName, req)

		require.NoError(t, err)
		require.NotNil(t, created, "expected CreateVersion to be called")
		assert.Equal(t, "sha256:abc", created.ImageId)
		assert.Equal(t, kindID, created.AgentKindID)
	})
}

// -----------------------------------------------------------------------------
// ValidateKindConfigValues — a pure function with no dependencies at all; the
// simplest unit-test shape, included to show the spectrum.
// -----------------------------------------------------------------------------

func TestValidateKindConfigValues(t *testing.T) {
	defaultVal := "fallback"
	schema := []models.KindConfigSchemaItem{
		{Name: "API_KEY", IsMandatory: true},
		{Name: "REGION", IsMandatory: true, DefaultValue: &defaultVal},
		{Name: "DEBUG", IsMandatory: false},
	}

	t.Run("passes when all mandatory values are supplied", func(t *testing.T) {
		err := ValidateKindConfigValues(schema, []spec.EnvironmentVariable{
			{Key: "API_KEY", Value: strPtr("secret")},
		})
		// REGION is covered by its default; DEBUG is optional.
		require.NoError(t, err)
	})

	t.Run("fails when a mandatory value without a default is missing", func(t *testing.T) {
		err := ValidateKindConfigValues(schema, nil)
		assert.ErrorIs(t, err, utils.ErrMissingKindConfigValue)
	})

	t.Run("treats an empty string as missing", func(t *testing.T) {
		err := ValidateKindConfigValues(schema, []spec.EnvironmentVariable{
			{Key: "API_KEY", Value: strPtr("")},
		})
		assert.ErrorIs(t, err, utils.ErrMissingKindConfigValue)
	})
}

// -----------------------------------------------------------------------------
// Labels — ListKinds filter passthrough, UpdateKind set/preserve/clear
// semantics, and PublishKind stamping labels onto a newly created kind.
// -----------------------------------------------------------------------------

func TestAgentKindService_Labels(t *testing.T) {
	const org, kindName = "acme", "chatbot"

	t.Run("ListKinds passes the label filter to the repo and surfaces labels", func(t *testing.T) {
		var gotFilter map[string]string
		repo := &repomocks.AgentKindRepositoryMock{
			ListKindsFunc: func(_ context.Context, _ string, labelFilter map[string]string, _, _ int) ([]models.AgentKind, int64, error) {
				gotFilter = labelFilter
				return []models.AgentKind{{Name: kindName, Labels: map[string]string{"env": "prod"}}}, 1, nil
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		resp, err := svc.ListKinds(context.Background(), org, map[string]string{"env": "prod"}, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod"}, gotFilter)
		require.Len(t, resp.Kinds, 1)
		assert.Equal(t, map[string]string{"env": "prod"}, resp.Kinds[0].Labels)
		assert.Equal(t, int64(1), resp.Total)
	})

	t.Run("UpdateKind replaces labels when the request provides them", func(t *testing.T) {
		var persisted map[string]string
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return &models.AgentKind{Name: kindName, DisplayName: "Chatbot", Labels: map[string]string{"old": "label"}}, nil
			},
			UpdateKindFunc: func(_ context.Context, kind *models.AgentKind) error {
				persisted = kind.Labels
				return nil
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		newLabels := map[string]string{"env": "prod", "team": "ml"}
		resp, err := svc.UpdateKind(context.Background(), org, kindName, &spec.UpdateAgentKindRequest{
			DisplayName: "Chatbot",
			Labels:      &newLabels,
		})

		require.NoError(t, err)
		assert.Equal(t, newLabels, persisted)
		assert.Equal(t, newLabels, resp.Labels)
	})

	t.Run("UpdateKind preserves labels when the request omits them", func(t *testing.T) {
		existing := map[string]string{"env": "prod"}
		var persisted map[string]string
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return &models.AgentKind{Name: kindName, DisplayName: "Chatbot", Labels: existing}, nil
			},
			UpdateKindFunc: func(_ context.Context, kind *models.AgentKind) error {
				persisted = kind.Labels
				return nil
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		_, err := svc.UpdateKind(context.Background(), org, kindName, &spec.UpdateAgentKindRequest{
			DisplayName: "Chatbot",
		})

		require.NoError(t, err)
		assert.Equal(t, existing, persisted)
	})

	t.Run("UpdateKind clears labels when the request sends an empty map", func(t *testing.T) {
		var persisted map[string]string
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return &models.AgentKind{Name: kindName, DisplayName: "Chatbot", Labels: map[string]string{"env": "prod"}}, nil
			},
			UpdateKindFunc: func(_ context.Context, kind *models.AgentKind) error {
				persisted = kind.Labels
				return nil
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		empty := map[string]string{}
		_, err := svc.UpdateKind(context.Background(), org, kindName, &spec.UpdateAgentKindRequest{
			DisplayName: "Chatbot",
			Labels:      &empty,
		})

		require.NoError(t, err)
		assert.Empty(t, persisted)
	})

	t.Run("PublishKind stamps kindLabels onto a newly created kind", func(t *testing.T) {
		var created *models.AgentKind
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return nil, gorm.ErrRecordNotFound
			},
			// Failing the create short-circuits publishVersion; the assertion
			// only cares about the labels handed to the repo.
			CreateKindFunc: func(_ context.Context, kind *models.AgentKind) error {
				created = kind
				return errors.New("stop after capture")
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		labels := map[string]string{"category": "assistant"}
		_, err := svc.PublishKind(context.Background(), org, "proj", "agent", &spec.PublishAgentKindRequest{
			KindName:   kindName,
			Version:    "v1.0.0",
			BuildName:  "build-1",
			KindLabels: &labels,
		})

		require.Error(t, err)
		require.NotNil(t, created)
		assert.Equal(t, labels, created.Labels)
	})

	t.Run("PublishKind normalizes absent kindLabels to an empty map", func(t *testing.T) {
		var created *models.AgentKind
		repo := &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
				return nil, gorm.ErrRecordNotFound
			},
			CreateKindFunc: func(_ context.Context, kind *models.AgentKind) error {
				created = kind
				return errors.New("stop after capture")
			},
		}
		svc := newKindService(repo, &clientmocks.OpenChoreoClientMock{})

		_, err := svc.PublishKind(context.Background(), org, "proj", "agent", &spec.PublishAgentKindRequest{
			KindName:  kindName,
			Version:   "v1.0.0",
			BuildName: "build-1",
		})

		require.Error(t, err)
		require.NotNil(t, created)
		assert.NotNil(t, created.Labels)
		assert.Empty(t, created.Labels)
	})
}
