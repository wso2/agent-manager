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
	"fmt"

	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

// ImportLegacySecret inserts a tenant-scoped copy of a legacy managed secret.
// A matching row is a successful retry. A different row is preserved and reported.
func (c *Client) ImportLegacySecret(ctx context.Context, ouID, name string, data map[string]string) (bool, error) {
	if ouID == "" || ouID == platformSecretScope || len(validation.IsValidLabelValue(ouID)) > 0 ||
		len(validation.IsDNS1123Subdomain(name)) > 0 {
		return false, fmt.Errorf("invalid destination organization or secret name")
	}
	ciphertext, hash, keys, err := c.seal(ctx, ouID, name, data)
	if err != nil {
		return false, err
	}
	const owner = secretmanagersvc.DefaultManagedBy
	row := secretRow{
		OuID: ouID, Name: name, Ciphertext: ciphertext, Hash: hash, Keys: keys,
		Labels: map[string]string{secretmanagersvc.LabelKeyManagedBy: owner}, ManagedBy: owner,
		Provider: c.vault.ProviderName(),
	}
	result := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "ou_id"}, {Name: "name"}}, DoNothing: true,
	}).Create(&row)
	if result.Error != nil {
		return false, fmt.Errorf("insert imported secret: %w", result.Error)
	}
	if result.RowsAffected != 0 {
		return true, nil
	}
	existing, err := c.find(ctx, c.db, ouID, name, false)
	if err != nil {
		return false, err
	}
	if existing.ManagedBy != owner || existing.Hash != hash {
		return false, fmt.Errorf("destination already contains a different secret")
	}
	return false, nil
}
