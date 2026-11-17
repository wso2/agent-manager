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

// Package inhouse is the agent manager's own secret store. Each secret is one
// row of the secrets table holding all of its keys, and its value goes through
// the deployment's vault: encrypted into the row by the local vault, or kept
// in an external vault that the row refers to.
package inhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ProviderName is the name the provider is selected by.
const ProviderName = "inhouse"

// ownerKind marks the secrets rows this provider owns. Such a row is its own
// owner: nothing else refers to it by id.
const ownerKind = "named_secret"

// MaxSecretKeys bounds the keys of one secret and MaxSecretBytes the combined
// size of its keys and values, so an accidental bulk write is refused.
const (
	MaxSecretKeys  = 100
	MaxSecretBytes = 256 << 10
)

// idNamespace scopes the row ids derived from secret names.
var idNamespace = uuid.MustParse("6f1d1f0a-6f0e-4d0b-9a3b-2f1c9b7e4a10")

type provider struct{}

// NewProvider returns the in-house secret store provider.
func NewProvider() secretmanagersvc.Provider { return provider{} }

func (provider) Capabilities() secretmanagersvc.StoreCapabilities {
	return secretmanagersvc.StoreCapabilityReadWrite
}

func (provider) ValidateConfig(config *secretmanagersvc.StoreConfig) error {
	if config == nil {
		return errors.New("config is required")
	}
	if config.DB == nil || config.Secrets == nil {
		return errors.New("the in-house secret store needs the database and its secret store")
	}
	return nil
}

func (p provider) NewClient(config *secretmanagersvc.StoreConfig) (secretmanagersvc.SecretsClient, error) {
	if err := p.ValidateConfig(config); err != nil {
		return nil, err
	}
	return &client{db: config.DB, rows: config.Secrets}, nil
}

type client struct {
	db   *gorm.DB
	rows secretmanagersvc.SecretRows
}

// rowID is the secrets row of the secret named name in org. Deriving it from
// the name makes the name unique per organization through the primary key.
func rowID(org, name string) (uuid.UUID, error) {
	if strings.TrimSpace(org) == "" || strings.TrimSpace(name) == "" {
		return uuid.Nil, fmt.Errorf("%w: a secret needs an organization and a name", utils.ErrInvalidInput)
	}
	return uuid.NewSHA1(idNamespace, []byte(org+"/"+name)), nil
}

func encode(data map[string]string) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: secret data cannot be empty", utils.ErrInvalidInput)
	}
	if len(data) > MaxSecretKeys {
		return nil, fmt.Errorf("%w: a secret holds at most %d keys, got %d", utils.ErrInvalidInput, MaxSecretKeys, len(data))
	}
	size := 0
	for k, v := range data {
		size += len(k) + len(v)
	}
	if size > MaxSecretBytes {
		return nil, fmt.Errorf("%w: a secret holds at most %d bytes of keys and values, got %d", utils.ErrInvalidInput, MaxSecretBytes, size)
	}
	return json.Marshal(data)
}

func (c *client) load(ctx context.Context, tx *gorm.DB, id uuid.UUID) (map[string]string, error) {
	payload, found, err := c.rows.Read(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, secretmanagersvc.ErrSecretNotFound
	}
	data := map[string]string{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("failed to decode secret: %w", err)
	}
	return data, nil
}

func (c *client) store(ctx context.Context, tx *gorm.DB, org string, id uuid.UUID, data map[string]string) error {
	payload, err := encode(data)
	if err != nil {
		return err
	}
	return c.rows.Write(ctx, tx, org, ownerKind, id, id, payload)
}

// PushSecret replaces all of the secret's data, creating it when absent.
func (c *client) PushSecret(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, _ *secretmanagersvc.SecretMetadata) (string, error) {
	name := location.SecretRefName()
	id, err := rowID(location.OrgName, name)
	if err != nil {
		return "", err
	}
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return c.store(ctx, tx, location.OrgName, id, data)
	})
	if err != nil {
		return "", err
	}
	return name, nil
}

// PatchSecret merges data into an existing secret and removes keysToDelete.
func (c *client) PatchSecret(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, keysToDelete []string, _ *secretmanagersvc.SecretMetadata) (string, error) {
	name := location.SecretRefName()
	id, err := rowID(location.OrgName, name)
	if err != nil {
		return "", err
	}
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dbx.LockRow(tx, models.Secret{}.TableName(), "version", "id = ?", id); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return secretmanagersvc.ErrSecretNotFound
			}
			return err
		}
		merged, err := c.load(ctx, tx, id)
		if err != nil {
			return err
		}
		maps.Copy(merged, data)
		for _, k := range keysToDelete {
			delete(merged, k)
		}
		return c.store(ctx, tx, location.OrgName, id, merged)
	})
	if err != nil {
		return "", err
	}
	return name, nil
}

// DeleteSecret removes the secret. Idempotent.
func (c *client) DeleteSecret(ctx context.Context, location secretmanagersvc.SecretLocation, metadata *secretmanagersvc.SecretMetadata) error {
	return c.DeleteSecretByName(ctx, location.OrgName, location.SecretRefName(), metadata)
}

// DeleteSecretByName removes the secret named secretRefName. Idempotent.
func (c *client) DeleteSecretByName(ctx context.Context, orgName, secretRefName string, _ *secretmanagersvc.SecretMetadata) error {
	id, err := rowID(orgName, secretRefName)
	if err != nil {
		return err
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return c.rows.Delete(ctx, tx, id)
	})
}

// GetSecret returns the secret's keys without its values.
func (c *client) GetSecret(ctx context.Context, location secretmanagersvc.SecretLocation) (*secretmanagersvc.SecretInfo, error) {
	name := location.SecretRefName()
	id, err := rowID(location.OrgName, name)
	if err != nil {
		return nil, err
	}
	data, err := c.load(ctx, c.db, id)
	if err != nil {
		return nil, err
	}
	return &secretmanagersvc.SecretInfo{
		ID:     name,
		Name:   name,
		Keys:   slices.Sorted(maps.Keys(data)),
		Labels: map[string]string{secretmanagersvc.LabelKeyManagedBy: secretmanagersvc.DefaultManagedBy},
	}, nil
}

// Close releases nothing; the database handle is owned by the caller.
func (c *client) Close(context.Context) error { return nil }
