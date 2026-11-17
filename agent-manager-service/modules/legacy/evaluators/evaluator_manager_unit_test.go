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

// UNIT tests for evaluatorManagerService. No `//go:build integration` tag, so
// this runs in the fast unit tier with all repository dependencies mocked via
// repomocks (moq-generated). The real built-in catalog package is used as-is
// (it is an in-memory, pure-Go table), which lets us exercise the merge/fallback
// branches without a database. An unconfigured mock method panics, so any test
// that hits an unexpected repo call fails loudly. Sentinel errors are asserted
// with assert.ErrorIs (never == or string match).
package evaluators

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy/evaluators/evalrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy/evaluators/evalrepo/evalmocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// discardLogger returns a slog.Logger that drops all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newEvaluatorService wires the service with the two mocked repositories.
func newEvaluatorService(cust *evalmocks.CustomEvaluatorRepositoryMock, mon *evalmocks.MonitorRepositoryMock) EvaluatorManagerService {
	return NewEvaluatorManagerService(discardLogger(), cust, mon)
}

// -----------------------------------------------------------------------------
// ListEvaluators — merge of custom + built-in, provider->type mapping, source
// gating, pagination, and repo-error propagation.
// -----------------------------------------------------------------------------

func TestEvaluatorManagerService_ListEvaluators(t *testing.T) {
	const org = "acme"

	t.Run("propagates custom repo error (does not swallow)", func(t *testing.T) {
		boom := errors.New("db down")
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			ListFunc: func(_ string, _ evalrepo.CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
				return nil, 0, boom
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, _, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("source=builtin skips the custom repo entirely", func(t *testing.T) {
		// Leaving ListFunc nil asserts the custom repo must NOT be called.
		cust := &evalmocks.CustomEvaluatorRepositoryMock{}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		page, total, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{Source: "builtin"})

		require.NoError(t, err)
		assert.Greater(t, total, int32(0), "expected built-in evaluators in the catalog")
		assert.Len(t, page, int(total))
		for _, e := range page {
			assert.True(t, e.IsBuiltin)
		}
	})

	t.Run("source=custom skips the built-in catalog and returns only customs", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			ListFunc: func(_ string, _ evalrepo.CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
				return []models.CustomEvaluator{
					{Identifier: "my-eval", DisplayName: "My Eval", Type: models.CustomEvaluatorTypeCode},
				}, 1, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		page, total, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{Source: "custom"})

		require.NoError(t, err)
		assert.Equal(t, int32(1), total)
		require.Len(t, page, 1)
		assert.False(t, page[0].IsBuiltin)
		assert.Equal(t, "my-eval", page[0].Identifier)
	})

	t.Run("maps custom_code provider filter to code type", func(t *testing.T) {
		var seen evalrepo.CustomEvaluatorFilters
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			ListFunc: func(_ string, f evalrepo.CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
				seen = f
				return nil, 0, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, _, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{
			Source:   "custom",
			Provider: models.CustomProviderCode,
		})

		require.NoError(t, err)
		assert.Equal(t, models.CustomEvaluatorTypeCode, seen.Type)
	})

	t.Run("maps custom_llm_judge provider filter to llm_judge type", func(t *testing.T) {
		var seen evalrepo.CustomEvaluatorFilters
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			ListFunc: func(_ string, f evalrepo.CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
				seen = f
				return nil, 0, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, _, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{
			Source:   "custom",
			Provider: models.CustomProviderLLMJudge,
		})

		require.NoError(t, err)
		assert.Equal(t, models.CustomEvaluatorTypeLLMJudge, seen.Type)
	})

	t.Run("merges customs first then built-ins", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			ListFunc: func(_ string, _ evalrepo.CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
				return []models.CustomEvaluator{
					{Identifier: "custom-one", DisplayName: "Custom One", Type: models.CustomEvaluatorTypeCode},
				}, 1, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		page, total, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{})

		require.NoError(t, err)
		assert.Greater(t, total, int32(1))
		require.NotEmpty(t, page)
		// Custom evaluators are prepended before built-ins.
		assert.Equal(t, "custom-one", page[0].Identifier)
		assert.False(t, page[0].IsBuiltin)
	})

	t.Run("applies limit and offset pagination over the merged set", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			ListFunc: func(_ string, _ evalrepo.CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
				return []models.CustomEvaluator{
					{Identifier: "c0", DisplayName: "C0", Type: models.CustomEvaluatorTypeCode},
					{Identifier: "c1", DisplayName: "C1", Type: models.CustomEvaluatorTypeCode},
					{Identifier: "c2", DisplayName: "C2", Type: models.CustomEvaluatorTypeCode},
				}, 3, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		page, total, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{
			Source: "custom",
			Limit:  2,
			Offset: 1,
		})

		require.NoError(t, err)
		assert.Equal(t, int32(3), total, "total reflects full merged set, not the page")
		require.Len(t, page, 2)
		assert.Equal(t, "c1", page[0].Identifier)
		assert.Equal(t, "c2", page[1].Identifier)
	})

	t.Run("offset beyond the end yields an empty page but real total", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			ListFunc: func(_ string, _ evalrepo.CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
				return []models.CustomEvaluator{
					{Identifier: "c0", DisplayName: "C0", Type: models.CustomEvaluatorTypeCode},
				}, 1, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		page, total, err := svc.ListEvaluators(context.Background(), org, EvaluatorFilters{
			Source: "custom",
			Offset: 100,
		})

		require.NoError(t, err)
		assert.Equal(t, int32(1), total)
		assert.Empty(t, page)
	})
}

// -----------------------------------------------------------------------------
// GetEvaluator — built-in lookup short-circuits; custom fallback with not-found
// and real-error branches.
// -----------------------------------------------------------------------------

func TestEvaluatorManagerService_GetEvaluator(t *testing.T) {
	const org = "acme"

	t.Run("returns a built-in evaluator without touching the custom repo", func(t *testing.T) {
		// GetByIdentifierFunc nil => custom repo must NOT be reached.
		cust := &evalmocks.CustomEvaluatorRepositoryMock{}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		resp, err := svc.GetEvaluator(context.Background(), org, "length_compliance")

		require.NoError(t, err)
		assert.True(t, resp.IsBuiltin)
		assert.Equal(t, "length_compliance", resp.Identifier)
	})

	t.Run("falls back to custom evaluator when not built-in", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, identifier string) (*models.CustomEvaluator, error) {
				return &models.CustomEvaluator{Identifier: identifier, DisplayName: "Custom", Type: models.CustomEvaluatorTypeCode}, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		resp, err := svc.GetEvaluator(context.Background(), org, "my-custom")

		require.NoError(t, err)
		assert.False(t, resp.IsBuiltin)
		assert.Equal(t, "my-custom", resp.Identifier)
	})

	t.Run("maps record-not-found to ErrEvaluatorNotFound", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.GetEvaluator(context.Background(), org, "missing")

		assert.ErrorIs(t, err, utils.ErrEvaluatorNotFound)
	})

	t.Run("propagates an unexpected repo error (not masked as not-found)", func(t *testing.T) {
		boom := errors.New("connection reset")
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return nil, boom
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.GetEvaluator(context.Background(), org, "missing")

		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrEvaluatorNotFound)
		assert.ErrorIs(t, err, boom)
	})
}

// -----------------------------------------------------------------------------
// CreateCustomEvaluator — slug generation, identifier validation, built-in
// clash, unique-constraint mapping, happy path, and config/tag defaulting.
// -----------------------------------------------------------------------------

func TestEvaluatorManagerService_CreateCustomEvaluator(t *testing.T) {
	const org = "acme"

	t.Run("rejects an invalid identifier slug", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.CreateCustomEvaluator(context.Background(), org, &models.CreateCustomEvaluatorRequest{
			Identifier:  "Bad Identifier!",
			DisplayName: "Bad",
			Type:        models.CustomEvaluatorTypeCode,
		})

		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("rejects an identifier that clashes with a built-in", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.CreateCustomEvaluator(context.Background(), org, &models.CreateCustomEvaluatorRequest{
			Identifier:  "accuracy", // collides with a built-in (valid slug, so passes regex)
			DisplayName: "Accuracy",
			Type:        models.CustomEvaluatorTypeCode,
		})

		assert.ErrorIs(t, err, utils.ErrCustomEvaluatorIdentifierTaken)
	})

	t.Run("maps the unique-constraint violation to AlreadyExists", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			CreateFunc: func(_ *models.CustomEvaluator) error {
				return errors.New(`duplicate key value violates unique constraint "uq_custom_evaluator_org_identifier"`)
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.CreateCustomEvaluator(context.Background(), org, &models.CreateCustomEvaluatorRequest{
			Identifier:  "dup-eval",
			DisplayName: "Dup",
			Type:        models.CustomEvaluatorTypeCode,
		})

		assert.ErrorIs(t, err, utils.ErrCustomEvaluatorAlreadyExists)
	})

	t.Run("propagates a generic create error", func(t *testing.T) {
		boom := errors.New("disk full")
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			CreateFunc: func(_ *models.CustomEvaluator) error { return boom },
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.CreateCustomEvaluator(context.Background(), org, &models.CreateCustomEvaluatorRequest{
			Identifier:  "x-eval",
			DisplayName: "X",
			Type:        models.CustomEvaluatorTypeCode,
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, utils.ErrCustomEvaluatorAlreadyExists)
	})

	t.Run("generates a slug identifier from display name and defaults config/tags", func(t *testing.T) {
		var created *models.CustomEvaluator
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			CreateFunc: func(e *models.CustomEvaluator) error {
				created = e
				return nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		resp, err := svc.CreateCustomEvaluator(context.Background(), org, &models.CreateCustomEvaluatorRequest{
			DisplayName: "My Cool Evaluator",
			Type:        models.CustomEvaluatorTypeCode,
			Level:       "trace",
			Source:      "print('ok')",
		})

		require.NoError(t, err)
		require.NotNil(t, created, "expected Create to be called")
		assert.Equal(t, "my-cool-evaluator", created.Identifier)
		assert.Equal(t, org, created.OUID)
		assert.NotNil(t, created.ConfigSchema, "nil config schema must be defaulted to empty slice")
		assert.NotNil(t, created.Tags, "nil tags must be defaulted to empty slice")
		assert.Equal(t, "my-cool-evaluator", resp.Identifier)
	})
}

// -----------------------------------------------------------------------------
// GetCustomEvaluator — not-found mapping vs real error vs happy path.
// -----------------------------------------------------------------------------

func TestEvaluatorManagerService_GetCustomEvaluator(t *testing.T) {
	const org = "acme"

	t.Run("maps record-not-found to ErrCustomEvaluatorNotFound", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.GetCustomEvaluator(context.Background(), org, "missing")

		assert.ErrorIs(t, err, utils.ErrCustomEvaluatorNotFound)
	})

	t.Run("propagates an unexpected repo error", func(t *testing.T) {
		boom := errors.New("timeout")
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return nil, boom
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.GetCustomEvaluator(context.Background(), org, "x")

		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrCustomEvaluatorNotFound)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("returns the mapped response on success", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, identifier string) (*models.CustomEvaluator, error) {
				return &models.CustomEvaluator{Identifier: identifier, DisplayName: "OK", Type: models.CustomEvaluatorTypeCode}, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		resp, err := svc.GetCustomEvaluator(context.Background(), org, "ok-eval")

		require.NoError(t, err)
		assert.Equal(t, "ok-eval", resp.Identifier)
		assert.False(t, resp.IsBuiltin)
	})
}

// -----------------------------------------------------------------------------
// UpdateCustomEvaluator — not-found gate, selective pointer-field application,
// update-error propagation.
// -----------------------------------------------------------------------------

func TestEvaluatorManagerService_UpdateCustomEvaluator(t *testing.T) {
	const org = "acme"

	t.Run("maps record-not-found to ErrCustomEvaluatorNotFound", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.UpdateCustomEvaluator(context.Background(), org, "missing", &models.UpdateCustomEvaluatorRequest{})

		assert.ErrorIs(t, err, utils.ErrCustomEvaluatorNotFound)
	})

	t.Run("applies only the supplied (non-nil) fields and persists", func(t *testing.T) {
		existing := &models.CustomEvaluator{
			Identifier:  "ev",
			DisplayName: "Old Name",
			Description: "old desc",
			Type:        models.CustomEvaluatorTypeCode,
		}
		var saved *models.CustomEvaluator
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return existing, nil
			},
			UpdateFunc: func(e *models.CustomEvaluator) error {
				saved = e
				return nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		newName := "New Name"
		resp, err := svc.UpdateCustomEvaluator(context.Background(), org, "ev", &models.UpdateCustomEvaluatorRequest{
			DisplayName: &newName,
			// Description left nil => must be preserved.
		})

		require.NoError(t, err)
		require.NotNil(t, saved)
		assert.Equal(t, "New Name", saved.DisplayName)
		assert.Equal(t, "old desc", saved.Description, "nil field must not be overwritten")
		assert.Equal(t, "New Name", resp.DisplayName)
	})

	t.Run("propagates an update error", func(t *testing.T) {
		boom := errors.New("write conflict")
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return &models.CustomEvaluator{Identifier: "ev", Type: models.CustomEvaluatorTypeCode}, nil
			},
			UpdateFunc: func(_ *models.CustomEvaluator) error { return boom },
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.UpdateCustomEvaluator(context.Background(), org, "ev", &models.UpdateCustomEvaluatorRequest{})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

// -----------------------------------------------------------------------------
// DeleteCustomEvaluator — not-found gate, in-use guard via monitor repo,
// usage-check error, and happy path.
// -----------------------------------------------------------------------------

func TestEvaluatorManagerService_DeleteCustomEvaluator(t *testing.T) {
	const org = "acme"

	t.Run("maps record-not-found to ErrCustomEvaluatorNotFound", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		err := svc.DeleteCustomEvaluator(context.Background(), org, "missing")

		assert.ErrorIs(t, err, utils.ErrCustomEvaluatorNotFound)
	})

	t.Run("refuses deletion when referenced by active monitors", func(t *testing.T) {
		// SoftDeleteFunc nil => must NOT be reached.
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return &models.CustomEvaluator{Identifier: "ev"}, nil
			},
		}
		mon := &evalmocks.MonitorRepositoryMock{
			FindActiveMonitorsByEvaluatorIdentifierFunc: func(_ string, _ string) ([]models.Monitor, error) {
				return []models.Monitor{{}}, nil
			},
		}
		svc := newEvaluatorService(cust, mon)

		err := svc.DeleteCustomEvaluator(context.Background(), org, "ev")

		assert.ErrorIs(t, err, utils.ErrCustomEvaluatorInUse)
	})

	t.Run("propagates a usage-check error", func(t *testing.T) {
		boom := errors.New("monitor query failed")
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return &models.CustomEvaluator{Identifier: "ev"}, nil
			},
		}
		mon := &evalmocks.MonitorRepositoryMock{
			FindActiveMonitorsByEvaluatorIdentifierFunc: func(_ string, _ string) ([]models.Monitor, error) {
				return nil, boom
			},
		}
		svc := newEvaluatorService(cust, mon)

		err := svc.DeleteCustomEvaluator(context.Background(), org, "ev")

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("soft-deletes when no active monitors reference it", func(t *testing.T) {
		softDeleted := false
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifierFunc: func(_ string, _ string) (*models.CustomEvaluator, error) {
				return &models.CustomEvaluator{Identifier: "ev"}, nil
			},
			SoftDeleteFunc: func(_ *models.CustomEvaluator) error {
				softDeleted = true
				return nil
			},
		}
		mon := &evalmocks.MonitorRepositoryMock{
			FindActiveMonitorsByEvaluatorIdentifierFunc: func(_ string, _ string) ([]models.Monitor, error) {
				return []models.Monitor{}, nil
			},
		}
		svc := newEvaluatorService(cust, mon)

		err := svc.DeleteCustomEvaluator(context.Background(), org, "ev")

		require.NoError(t, err)
		assert.True(t, softDeleted, "expected SoftDelete to be called")
	})
}

// -----------------------------------------------------------------------------
// ResolveCustomEvaluators — empty-input short-circuit and delegation.
// -----------------------------------------------------------------------------

func TestEvaluatorManagerService_ResolveCustomEvaluators(t *testing.T) {
	const org = "acme"

	t.Run("returns nil for empty identifiers without hitting the repo", func(t *testing.T) {
		// GetByIdentifiersFunc nil => must NOT be reached.
		cust := &evalmocks.CustomEvaluatorRepositoryMock{}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		out, err := svc.ResolveCustomEvaluators(context.Background(), org, nil)

		require.NoError(t, err)
		assert.Nil(t, out)
	})

	t.Run("delegates to the repo for non-empty identifiers", func(t *testing.T) {
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifiersFunc: func(_ string, ids []string) ([]models.CustomEvaluator, error) {
				return []models.CustomEvaluator{{Identifier: ids[0]}}, nil
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		out, err := svc.ResolveCustomEvaluators(context.Background(), org, []string{"a", "b"})

		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, "a", out[0].Identifier)
	})

	t.Run("propagates a repo error", func(t *testing.T) {
		boom := errors.New("batch fetch failed")
		cust := &evalmocks.CustomEvaluatorRepositoryMock{
			GetByIdentifiersFunc: func(_ string, _ []string) ([]models.CustomEvaluator, error) {
				return nil, boom
			},
		}
		svc := newEvaluatorService(cust, &evalmocks.MonitorRepositoryMock{})

		_, err := svc.ResolveCustomEvaluators(context.Background(), org, []string{"a"})

		assert.ErrorIs(t, err, boom)
	})
}

func TestValidateEvaluatorConfigSchema(t *testing.T) {
	t.Run("allows negative bounds for numeric parameters", func(t *testing.T) {
		min, max := -10.0, -1.0

		err := validateEvaluatorConfigSchema([]models.EvaluatorConfigParam{{
			Key:  "score_offset",
			Type: "integer",
			Min:  &min,
			Max:  &max,
		}})

		assert.NoError(t, err)
	})

	t.Run("rejects a minimum greater than the maximum", func(t *testing.T) {
		min, max := 10.0, 1.0

		err := validateEvaluatorConfigSchema([]models.EvaluatorConfigParam{{
			Key:  "threshold",
			Type: "float",
			Min:  &min,
			Max:  &max,
		}})

		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("rejects fractional bounds for integer parameters", func(t *testing.T) {
		min := 1.5

		err := validateEvaluatorConfigSchema([]models.EvaluatorConfigParam{{
			Key:  "max_retries",
			Type: "integer",
			Min:  &min,
		}})

		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})
}
