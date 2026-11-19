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
	"github.com/wso2/agent-manager/agent-manager-service/vault"
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

// emptySegment stands for an unset scope field in a secret's name. KV path segments can never be ".", so it cannot
// be mistaken for a project, environment, agent or config of that name.
const emptySegment = "."

// secretName is the reference of the secret at location: its organization, project, environment, agent, config and
// entity, without the key, since one row holds all of a secret's keys. Every field keeps its position, an unset one
// written as emptySegment, so two locations differing in which optional field is set never share a name. The six
// segments parse back through secretmanagersvc.ParseKVPath to the same location, so a reference this provider returns
// finds the same row. SecretRefName is not used: it leaves out the project and agent and is cut to 63 characters.
func secretName(location secretmanagersvc.SecretLocation) (string, error) {
	if strings.TrimSpace(location.OrgName) == "" || strings.TrimSpace(location.EntityName) == "" {
		return "", fmt.Errorf("%w: a secret needs an organization and an entity", utils.ErrInvalidInput)
	}
	fields := []string{
		location.OrgName, location.ProjectName, location.EnvironmentName,
		location.AgentName, location.ConfigName, location.EntityName,
	}
	parts := make([]string, len(fields))
	for i, field := range fields {
		if field == emptySegment || strings.TrimSpace(field) == "" {
			parts[i] = emptySegment
			continue
		}
		segment, err := secretmanagersvc.SanitizeSegment(field)
		if err != nil {
			return "", fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
		}
		parts[i] = segment
	}
	if parts[0] == emptySegment || parts[5] == emptySegment {
		return "", fmt.Errorf("%w: a secret needs an organization and an entity", utils.ErrInvalidInput)
	}
	return strings.Join(parts, "/"), nil
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
	name, err := secretName(location)
	if err != nil {
		return "", err
	}
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
	name, err := secretName(location)
	if err != nil {
		return "", err
	}
	id, err := rowID(location.OrgName, name)
	if err != nil {
		return "", err
	}
	patch := func(merged map[string]string) {
		maps.Copy(merged, data)
		for _, k := range keysToDelete {
			delete(merged, k)
		}
	}
	for range patchAttempts {
		state, err := c.state(ctx, id)
		if err != nil {
			return "", err
		}
		// A locally sealed value opens without I/O, so it is read under the lock. An external one is read first, since
		// opening it calls the external vault; the lock is then taken only at the version read, and the patch starts
		// over if a write came in between.
		var external map[string]string
		if state.Provider != vault.ProviderLocal {
			if external, err = c.load(ctx, c.db.WithContext(ctx), id); err != nil {
				return "", err
			}
		}
		changed := false
		err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			where, args := "id = ?", []any{id}
			if external != nil {
				where, args = "id = ? AND version = ?", []any{id, state.Version}
			}
			err := dbx.LockRow(tx, models.Secret{}.TableName(), "version", where, args...)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				changed = true
				return nil
			}
			if err != nil {
				return fmt.Errorf("failed to lock secret %s: %w", id, err)
			}
			merged := external
			if merged == nil {
				// Re-check under the lock: a promotion may have made the value external since state was read.
				current, err := c.stateIn(tx, id)
				if err != nil {
					return err
				}
				if current.Provider != vault.ProviderLocal {
					changed = true
					return nil
				}
				if merged, err = c.load(ctx, tx, id); err != nil {
					return err
				}
			}
			patch(merged)
			return c.store(ctx, tx, location.OrgName, id, merged)
		})
		if err != nil {
			return "", err
		}
		if !changed {
			return name, nil
		}
	}
	return "", fmt.Errorf("secret %s kept changing during %d attempts to patch it", name, patchAttempts)
}

// patchAttempts bounds how often PatchSecret starts over because another write changed an external secret.
const patchAttempts = 5

// secretState is what PatchSecret checks of a secret before and after taking its lock.
type secretState struct {
	Version  int
	Provider string
}

// state returns the secret's current version and vault, or ErrSecretNotFound.
func (c *client) state(ctx context.Context, id uuid.UUID) (secretState, error) {
	return c.stateIn(c.db.WithContext(ctx), id)
}

func (c *client) stateIn(db *gorm.DB, id uuid.UUID) (secretState, error) {
	var states []secretState
	if err := db.Model(&models.Secret{}).Select("version", "provider").Where("id = ?", id).Find(&states).Error; err != nil {
		return secretState{}, fmt.Errorf("failed to read the state of secret %s: %w", id, err)
	}
	if len(states) == 0 {
		return secretState{}, secretmanagersvc.ErrSecretNotFound
	}
	return states[0], nil
}

// DeleteSecret removes the secret. Idempotent.
func (c *client) DeleteSecret(ctx context.Context, location secretmanagersvc.SecretLocation, metadata *secretmanagersvc.SecretMetadata) error {
	name, err := secretName(location)
	if err != nil {
		return err
	}
	return c.DeleteSecretByName(ctx, location.OrgName, name, metadata)
}

// DeleteSecretByName removes the secret whose reference is secretRefName, as
// PushSecret returned it. Idempotent.
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
	name, err := secretName(location)
	if err != nil {
		return nil, err
	}
	id, err := rowID(location.OrgName, name)
	if err != nil {
		return nil, err
	}
	data, err := c.load(ctx, c.db, id)
	if err != nil {
		return nil, err
	}
	return &secretmanagersvc.SecretInfo{
		ID:   name,
		Name: name,
		Keys: slices.Sorted(maps.Keys(data)),
		// The store does not report when a secret was created; the field is omitted from the JSON when empty.
		CreatedAt: "",
		Labels:    map[string]string{secretmanagersvc.LabelKeyManagedBy: secretmanagersvc.DefaultManagedBy},
	}, nil
}

// Close releases nothing; the database handle is owned by the caller.
func (c *client) Close(context.Context) error { return nil }
