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
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// NewEnvThunderSecretReader builds the resolver's DB-backed credential reader.
// Lives in services (not wiring) so app.Run's provisioning factory can share it without a cycle.
func NewEnvThunderSecretReader(repo repositories.EnvThunderSystemClientRepository, encryptionKey []byte) thundersvc.ReadSystemClientFunc {
	return func(ctx context.Context, ouID, envName string) (string, string, error) {
		row, err := repo.Get(ctx, ouID, envName)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if clientID, clientSecret, ok := platformThunderSystemClient(envName); ok {
				return clientID, clientSecret, nil
			}
			return "", "", thundersvc.ErrThunderNotProvisioned
		}
		if err != nil {
			return "", "", fmt.Errorf("read env-thunder system-client for %s/%s: %w", ouID, envName, err)
		}
		secret, err := utils.DecryptBytes(row.ClientSecretEncrypted, encryptionKey)
		if err != nil {
			return "", "", fmt.Errorf("decrypt env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
		}
		return row.ClientID, string(secret), nil
	}
}
