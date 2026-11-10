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

package inhouse

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	platformSecretScope = "platform"
	platformSecretName  = "bootstrap"
	platformSecretOwner = "amp-platform-bootstrap"
)

// SeedPlatformSecrets upserts the deployment's platform credentials before the
// webhook starts serving. The deployment Secret is the source of truth. An
// unchanged payload does not rewrite the row when replicas start or restart.
func (c *Client) SeedPlatformSecrets(ctx context.Context, seed string) error {
	if seed == "" {
		return nil
	}
	var data map[string]string
	if err := json.Unmarshal([]byte(seed), &data); err != nil || len(data) == 0 {
		return fmt.Errorf("AMP_PLATFORM_VAULT_SEED must be a non-empty JSON object of platform credentials")
	}
	for name, value := range data {
		switch name {
		case "opensearch-username", "opensearch-password", "observer-oauth-client-secret",
			"workflow-plane-oauth-client-secret", "amp-publisher-client-secret", "registry-push-secret":
			if value == "" {
				return fmt.Errorf("platform credential %q is empty", name)
			}
		default:
			return fmt.Errorf("AMP_PLATFORM_VAULT_SEED contains an unsupported key")
		}
	}
	ciphertext, hash, keys, err := c.seal(ctx, data)
	if err != nil {
		return err
	}
	row := secretRow{
		OuID: platformSecretScope, Name: platformSecretName, Ciphertext: ciphertext, Hash: hash,
		Keys: keys, Labels: map[string]string{}, ManagedBy: platformSecretOwner, Provider: c.vault.ProviderName(),
	}
	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "ou_id"}, {Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"ciphertext": ciphertext, "hash": hash, "keys": gorm.Expr("EXCLUDED.keys"),
			"provider": row.Provider, "updated_at": gorm.Expr("NOW()"),
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			gorm.Expr("secrets.managed_by = ? AND secrets.hash <> EXCLUDED.hash", platformSecretOwner),
		}},
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("seed platform credentials: %w", err)
	}
	return nil
}
