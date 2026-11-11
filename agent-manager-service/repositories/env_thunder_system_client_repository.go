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

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// EnvThunderSystemClientRepository defines data access for per-environment
// env-Thunder system-client credentials.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/env_thunder_system_client_repository_mock.go . EnvThunderSystemClientRepository:EnvThunderSystemClientRepositoryMock
type EnvThunderSystemClientRepository interface {
	// Get returns the credential for (ouID, envName), or
	// (nil, gorm.ErrRecordNotFound) if none exists.
	Get(ctx context.Context, ouID, envName string) (*models.EnvThunderSystemClient, error)
	// Upsert atomically creates or updates the credential for its (OUID, EnvName).
	Upsert(ctx context.Context, cred *models.EnvThunderSystemClient) error
	// Delete removes the credential for (ouID, envName). Deleting a
	// non-existent row is not an error.
	Delete(ctx context.Context, ouID, envName string) error
}

// envThunderSystemClientRepo stores no env-Thunder credential: Lite environments all use the platform Thunder.
type envThunderSystemClientRepo struct{}

// NewEnvThunderSystemClientRepo creates a new EnvThunderSystemClientRepository.
func NewEnvThunderSystemClientRepo(*gorm.DB) EnvThunderSystemClientRepository {
	return envThunderSystemClientRepo{}
}

func (envThunderSystemClientRepo) Get(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
	return nil, gorm.ErrRecordNotFound
}

func (envThunderSystemClientRepo) Upsert(context.Context, *models.EnvThunderSystemClient) error {
	return ErrEnvThunderUnsupported
}

func (envThunderSystemClientRepo) Delete(context.Context, string, string) error { return nil }
