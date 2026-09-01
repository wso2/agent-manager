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
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// ResolveThunderURL is the SINGLE place every caller resolves an
// environment's env-Thunder registration, so this logic can never drift
// apart between call sites. A missing row means never provisioned — returns
// (models.ThunderURLRecord{}, nil), never a value computed from (ouID, envName).
func ResolveThunderURL(ctx context.Context, urlRepo repositories.EnvThunderURLRepository, ouID, envName string) (models.ThunderURLRecord, error) {
	row, err := urlRepo.Get(ctx, ouID, envName)
	if err == nil {
		return toThunderURLRecord(row), nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ThunderURLRecord{}, nil
	}
	return models.ThunderURLRecord{}, fmt.Errorf("read env-thunder url for %s/%s: %w", ouID, envName, err)
}

// NewEnvThunderURLReader builds the resolver's DB-backed URL reader —
// ResolveThunderURL widened to thundersvc.ReadThunderURLFunc's shape.
// callerSupplied (Handle == "") tells the resolver whether this row's URL was
// supplied directly by the caller (attacker-influenced) or computed by AMS
// itself from a handle (its own trusted computation) — see
// thunderURLCandidate's doc comment for why that decides which candidate gets
// SSRF-hardened. Lives in services (not wiring), same as
// NewEnvThunderSecretReader, so app.Run's provisioning factory can share it
// without an import cycle.
func NewEnvThunderURLReader(urlRepo repositories.EnvThunderURLRepository) thundersvc.ReadThunderURLFunc {
	return func(ctx context.Context, ouID, envName string) (string, bool, error) {
		rec, err := ResolveThunderURL(ctx, urlRepo, ouID, envName)
		if err != nil {
			return "", false, err
		}
		return rec.URL, rec.Handle == "", nil
	}
}
