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
	"errors"
	"fmt"

	"gorm.io/gorm/clause"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	gitSecretOwner     = "amp-git-credentials"
	gitSecretNameLabel = "git-secret-name"
)

// CreateGitSecret inserts the tenant-scoped credential and publishes its typed
// reference for checkout workflows. Repeating a create repairs a missing
// reference but never replaces the existing credential.
func (c *Client) CreateGitSecret(ctx context.Context, ouID, name, username, password string) error {
	if err := validateOuID(ouID); err != nil {
		return err
	}
	ref := occlient.GitSecretReferenceName(ouID, name)
	if username == "" {
		username = "git"
	}
	ciphertext, hash, keys, err := c.seal(ctx, map[string]string{"username": username, "password": password})
	if err != nil {
		return err
	}
	row := secretRow{
		OuID: ouID, Name: ref, Ciphertext: ciphertext, Hash: hash, Keys: keys,
		Labels:    map[string]string{gitSecretNameLabel: name, secretmanagersvc.LabelKeyManagedBy: gitSecretOwner},
		ManagedBy: gitSecretOwner, Provider: c.vault.ProviderName(),
	}
	result := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "ou_id"}, {Name: "name"}}, DoNothing: true,
	}).Create(&row)
	if result.Error != nil {
		return fmt.Errorf("store Git secret: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		if err := c.CheckGitSecret(ctx, ouID, name); err != nil {
			return err
		}
	}
	req := occlient.CreateSecretReferenceRequest{
		Name: ref, KVPath: RemoteKey(ouID, ref), SecretKeys: keys,
		SecretType: "kubernetes.io/basic-auth", RefreshInterval: c.refreshInterval,
	}
	if _, err := c.oc.CreateSecretReference(ctx, ouID, req); err != nil {
		if !errors.Is(err, utils.ErrConflict) {
			return fmt.Errorf("publish Git secret reference: %w", err)
		}
		existing, err := c.oc.GetSecretReference(ctx, ouID, ref)
		if err != nil {
			return fmt.Errorf("verify Git secret reference: %w", err)
		}
		if existing == nil || len(existing.Data) != len(keys) {
			return secretmanagersvc.ErrReferenceNameTaken
		}
		for _, source := range existing.Data {
			if source.RemoteRef.Key != req.KVPath || source.RemoteRef.Property != source.SecretKey ||
				(source.SecretKey != "username" && source.SecretKey != "password") {
				return secretmanagersvc.ErrReferenceNameTaken
			}
		}
	}
	if result.RowsAffected == 0 {
		return utils.ErrGitSecretAlreadyExists
	}
	return nil
}

func (c *Client) ListGitSecretNames(ctx context.Context, ouID string, limit, offset int) ([]string, int, error) {
	if err := validateOuID(ouID); err != nil {
		return nil, 0, err
	}
	query := c.db.WithContext(ctx).Model(&secretRow{}).Where("ou_id = ? AND managed_by = ?", ouID, gitSecretOwner)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("count Git secrets: %w", err)
	}
	names := []string{}
	if err := query.Order("labels->>'git-secret-name'").Limit(limit).Offset(offset).Pluck("labels->>'git-secret-name'", &names).Error; err != nil {
		return nil, 0, fmt.Errorf("list Git secrets: %w", err)
	}
	return names, int(count), nil
}

func (c *Client) findGitSecret(ctx context.Context, ouID, name string) (*secretRow, error) {
	row, err := c.find(ctx, c.db, ouID, occlient.GitSecretReferenceName(ouID, name), false)
	if errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		return nil, utils.ErrGitSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.ManagedBy != gitSecretOwner || row.Labels[gitSecretNameLabel] != name {
		return nil, utils.ErrGitSecretNotFound
	}
	return row, nil
}

func (c *Client) CheckGitSecret(ctx context.Context, ouID, name string) error {
	_, err := c.findGitSecret(ctx, ouID, name)
	return err
}

func (c *Client) ReadGitSecret(ctx context.Context, ouID, name string) (map[string]string, error) {
	row, err := c.findGitSecret(ctx, ouID, name)
	if err != nil {
		return nil, err
	}
	return c.open(ctx, row)
}

func (c *Client) DeleteGitSecret(ctx context.Context, ouID, name string) error {
	if err := c.CheckGitSecret(ctx, ouID, name); err != nil {
		return err
	}
	return c.DeleteSecretByName(ctx, ouID, occlient.GitSecretReferenceName(ouID, name), &secretmanagersvc.SecretMetadata{
		ManagedBy: gitSecretOwner, Labels: nil,
	})
}
