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

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestLLMProviderRepo_Update_NameAndVersion(t *testing.T) {
	for _, tt := range []struct {
		name         string
		otherVersion string
		updateName   string
		wantConflict bool
	}{
		{name: "free target pair despite old version collision", otherVersion: "v1.0", updateName: "Provider A"},
		{name: "duplicate target pair rolls back", otherVersion: "v2.0", updateName: "Provider A", wantConflict: true},
		{name: "version only without a rename", otherVersion: "v1.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gdb := db.GetDB()
			repo := NewLLMProviderRepo(gdb)
			orgUUID := uuid.New().String()
			create := func(handle, name, version string) *models.LLMProvider {
				provider := &models.LLMProvider{
					Description:    "original description",
					TemplateHandle: "openai",
					Configuration: models.LLMProviderConfig{
						Name: name, Handle: handle, Version: version, Template: "openai",
					},
					Status: "CREATED",
				}
				require.NoError(t, repo.Create(gdb, provider, handle, name, version, orgUUID))
				t.Cleanup(func() {
					gdb.Exec("DELETE FROM llm_providers WHERE uuid = ?", provider.UUID)
					gdb.Exec("DELETE FROM artifacts WHERE uuid = ?", provider.UUID)
				})
				return provider
			}
			create("provider-a", "Provider A", tt.otherVersion)
			provider := create("provider-b", "Provider B", "v1.0")
			updates := &models.LLMProvider{
				Description:    "updated description",
				TemplateHandle: "openai",
				Configuration: models.LLMProviderConfig{
					Name: tt.updateName, Handle: "provider-b", Version: "v2.0", Template: "openai",
				},
			}

			err := repo.Update(context.Background(), updates, provider.UUID.String(), orgUUID)
			if tt.wantConflict {
				var pgErr *pgconn.PgError
				require.ErrorAs(t, err, &pgErr)
				require.Equal(t, "23505", pgErr.Code)
				require.Equal(t, "uq_artifact_name_version_ou_id", pgErr.ConstraintName)
			} else {
				require.NoError(t, err)
			}

			stored, err := repo.GetByUUID(provider.UUID.String(), orgUUID)
			require.NoError(t, err)
			require.Equal(t, "provider-b", stored.Artifact.Handle)
			if tt.wantConflict {
				require.Equal(t, "Provider B", stored.Artifact.Name)
				require.Equal(t, "v1.0", stored.Artifact.Version)
				require.Equal(t, provider.Configuration, stored.Configuration)
				require.Equal(t, provider.Description, stored.Description)
			} else {
				expectedName := tt.updateName
				if expectedName == "" {
					expectedName = "Provider B"
				}
				require.Equal(t, expectedName, stored.Artifact.Name)
				require.Equal(t, "v2.0", stored.Artifact.Version)
				require.Equal(t, updates.Configuration, stored.Configuration)
				require.Equal(t, updates.Description, stored.Description)
			}
		})
	}
}

// TestLLMProviderRepo_Update_SyncsArtifactName guards against a regression
// where renaming a provider updated configuration.name (an internal JSONB
// field) but left artifacts.name untouched. Every read path — both the list
// and detail LLM provider responses — serves the display name from
// artifacts.name (see utils.ConvertModelToSpecLLMProvider{ListItem}Response),
// so without this sync a rename silently never shows up anywhere in the API.
func TestLLMProviderRepo_Update_SyncsArtifactName(t *testing.T) {
	gdb := db.GetDB()
	repo := NewLLMProviderRepo(gdb)
	orgUUID := uuid.New().String()

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Configuration: models.LLMProviderConfig{
			Name:     "Original Name",
			Handle:   "rename-sync-test-provider",
			Version:  "v1.0",
			Template: "openai",
		},
		Status: "CREATED",
	}
	require.NoError(t, repo.Create(gdb, provider, provider.Configuration.Handle,
		provider.Configuration.Name, provider.Configuration.Version, orgUUID))

	t.Cleanup(func() {
		gdb.Exec("DELETE FROM llm_providers WHERE uuid = ?", provider.UUID)
		gdb.Exec("DELETE FROM artifacts WHERE uuid = ?", provider.UUID)
	})

	updates := &models.LLMProvider{
		Configuration: models.LLMProviderConfig{
			Name:     "Renamed Provider",
			Handle:   provider.Configuration.Handle,
			Version:  provider.Configuration.Version,
			Template: provider.Configuration.Template,
		},
	}
	require.NoError(t, repo.Update(context.Background(), updates, provider.UUID.String(), orgUUID))

	var artifact models.Artifact
	require.NoError(t, gdb.First(&artifact, "uuid = ?", provider.UUID).Error)
	require.Equal(t, "Renamed Provider", artifact.Name,
		"Update must sync the new name into artifacts.name — that's what every read path serves as the provider's display name")
}

// TestLLMProviderRepo_Update_EmptyNameLeavesArtifactNameUnchanged confirms an
// update that carries no name (Configuration.Name == "") does not blank out
// the artifact's existing name — ArtifactRepo.Update conditionally applies
// each field, so this only holds if callers keep passing "" rather than a
// literal empty-string overwrite.
func TestLLMProviderRepo_Update_EmptyNameLeavesArtifactNameUnchanged(t *testing.T) {
	gdb := db.GetDB()
	repo := NewLLMProviderRepo(gdb)
	orgUUID := uuid.New().String()

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Configuration: models.LLMProviderConfig{
			Name:     "Keep This Name",
			Handle:   "rename-sync-test-provider-2",
			Version:  "v1.0",
			Template: "openai",
		},
		Status: "CREATED",
	}
	require.NoError(t, repo.Create(gdb, provider, provider.Configuration.Handle,
		provider.Configuration.Name, provider.Configuration.Version, orgUUID))

	t.Cleanup(func() {
		gdb.Exec("DELETE FROM llm_providers WHERE uuid = ?", provider.UUID)
		gdb.Exec("DELETE FROM artifacts WHERE uuid = ?", provider.UUID)
	})

	updates := &models.LLMProvider{
		Description: "updated description only",
		Configuration: models.LLMProviderConfig{
			Handle:   provider.Configuration.Handle,
			Version:  provider.Configuration.Version,
			Template: provider.Configuration.Template,
		},
	}
	require.NoError(t, repo.Update(context.Background(), updates, provider.UUID.String(), orgUUID))

	var artifact models.Artifact
	require.NoError(t, gdb.First(&artifact, "uuid = ?", provider.UUID).Error)
	require.Equal(t, "Keep This Name", artifact.Name)
}
