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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	secretmanagersvc "github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/vault"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// secretRow is one stored secret: every key of one SecretReference, sealed together.
type secretRow struct {
	ID         string            `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	OuID       string            `gorm:"column:ou_id"`
	Name       string            `gorm:"column:name"`
	Ciphertext []byte            `gorm:"column:ciphertext"`
	Hash       string            `gorm:"column:hash"`
	Keys       []string          `gorm:"column:keys;serializer:json"`
	Labels     map[string]string `gorm:"column:labels;serializer:json"`
	ManagedBy  string            `gorm:"column:managed_by"`
	Provider   string            `gorm:"column:provider"`
	CreatedAt  time.Time         `gorm:"column:created_at"`
	UpdatedAt  time.Time         `gorm:"column:updated_at"`
}

func (secretRow) TableName() string { return "secrets" }

// Client stores secret values encrypted in the agent manager database. Workloads
// receive them through ExternalSecrets that name a secret's store key
// (RemoteKey) directly, which the in-house ClusterSecretStore resolves through
// Resolve.
type Client struct {
	db    *gorm.DB
	vault vault.SecretVault
	// maxBytes bounds one secret's keys and values (see seal).
	maxBytes int
}

var _ secretmanagersvc.SecretsClient = (*Client)(nil)

// RemoteKey is the remoteRef.key a SecretReference entry carries for a secret:
// "<ouID>/<name>". The ClusterSecretStore's webhook URL is built from it.
func RemoteKey(ouID, name string) string {
	return ouID + "/" + name
}

func validateMetadata(metadata *secretmanagersvc.SecretMetadata) error {
	if metadata == nil {
		return fmt.Errorf("secret metadata is required")
	}
	if strings.TrimSpace(metadata.ManagedBy) == "" {
		return fmt.Errorf("secret metadata managedBy is required")
	}
	return nil
}

func validateOuID(ouID string) error {
	if strings.TrimSpace(ouID) == "" || strings.ContainsAny(ouID, "/?#") {
		return fmt.Errorf("invalid organization %q for a secret", ouID)
	}
	return nil
}

// MaxSecretKeys bounds the keys of one secret, and DefaultMaxSecretBytes the
// combined size of its keys and values when the store is configured without a
// size (InHouseConfig.MaxSecretBytes). A workload typically holds about ten
// keys, so these reject accidental bulk writes. The size is counted on the raw
// keys and values, not their JSON encoding, so a value that escapes heavily in
// JSON is not refused below the size the caller was promised.
const (
	MaxSecretKeys         = 100
	DefaultMaxSecretBytes = 256 << 10
)

func secretPayloadBytes(data map[string]string) int {
	n := 0
	for k, v := range data {
		n += len(k) + len(v)
	}
	return n
}

func (c *Client) seal(ctx context.Context, data map[string]string) ([]byte, string, []string, error) {
	if len(data) == 0 {
		return nil, "", nil, errors.New("secret data cannot be empty")
	}
	if len(data) > MaxSecretKeys {
		return nil, "", nil, fmt.Errorf("%w: a secret holds at most %d keys, got %d", utils.ErrInvalidInput, MaxSecretKeys, len(data))
	}
	if size := secretPayloadBytes(data); size > c.maxBytes {
		return nil, "", nil, fmt.Errorf("%w: a secret holds at most %d bytes of keys and values, got %d", utils.ErrInvalidInput, c.maxBytes, size)
	}
	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to encode secret data: %w", err)
	}
	ciphertext, err := c.vault.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to encrypt secret: %w", err)
	}
	return ciphertext, c.vault.Hash(plaintext), slices.Sorted(maps.Keys(data)), nil
}

func (c *Client) open(ctx context.Context, row *secretRow) (map[string]string, error) {
	plaintext, err := c.vault.Decrypt(ctx, row.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret %q: %w", row.Name, err)
	}
	data := map[string]string{}
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("failed to decode secret %q: %w", row.Name, err)
	}
	return data, nil
}

func (c *Client) find(ctx context.Context, tx *gorm.DB, ouID, name string, lock bool) (*secretRow, error) {
	q := tx.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row secretRow
	if err := q.Where("ou_id = ? AND name = ?", ouID, name).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, secretmanagersvc.ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to read secret %q: %w", name, err)
	}
	return &row, nil
}

// write stores data as the secret's full contents, creating the row when absent.
// A row managed by another owner is never touched.
func (c *Client) write(ctx context.Context, location secretmanagersvc.SecretLocation, metadata *secretmanagersvc.SecretMetadata, merge func(existing map[string]string) (map[string]string, error)) (string, error) {
	if err := validateMetadata(metadata); err != nil {
		return "", err
	}
	if err := validateOuID(location.OrgName); err != nil {
		return "", err
	}
	name := location.SecretRefName()
	var keys []string
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := c.find(ctx, tx, location.OrgName, name, true)
		if err != nil && !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
			return err
		}
		var current map[string]string
		if existing != nil {
			if existing.ManagedBy != metadata.ManagedBy {
				return secretmanagersvc.ErrNotManaged
			}
			if current, err = c.open(ctx, existing); err != nil {
				return err
			}
		}
		data, err := merge(current)
		if err != nil {
			return err
		}
		ciphertext, hash, sortedKeys, err := c.seal(ctx, data)
		if err != nil {
			return err
		}
		keys = sortedKeys
		labels := maps.Clone(metadata.Labels)
		if labels == nil {
			labels = map[string]string{}
		}
		labels[secretmanagersvc.LabelKeyManagedBy] = metadata.ManagedBy
		if existing == nil {
			return tx.Create(&secretRow{
				OuID:       location.OrgName,
				Name:       name,
				Ciphertext: ciphertext,
				Hash:       hash,
				Keys:       keys,
				Labels:     labels,
				ManagedBy:  metadata.ManagedBy,
				Provider:   c.vault.ProviderName(),
			}).Error
		}
		existing.Ciphertext = ciphertext
		existing.Hash = hash
		existing.Keys = keys
		existing.Labels = labels
		existing.Provider = c.vault.ProviderName()
		existing.UpdatedAt = time.Now()
		return tx.Save(existing).Error
	})
	if err != nil {
		return "", err
	}
	return name, nil
}

// PushSecret replaces all of the secret's data, creating it when absent.
func (c *Client) PushSecret(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, metadata *secretmanagersvc.SecretMetadata) (string, error) {
	return c.write(ctx, location, metadata, func(map[string]string) (map[string]string, error) {
		return maps.Clone(data), nil
	})
}

// PatchSecret merges data into an existing secret and removes keysToDelete.
func (c *Client) PatchSecret(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, keysToDelete []string, metadata *secretmanagersvc.SecretMetadata) (string, error) {
	return c.write(ctx, location, metadata, func(existing map[string]string) (map[string]string, error) {
		if existing == nil {
			return nil, secretmanagersvc.ErrSecretNotFound
		}
		maps.Copy(existing, data)
		for _, k := range keysToDelete {
			delete(existing, k)
		}
		return existing, nil
	})
}

// DeleteSecret removes the secret. Idempotent.
func (c *Client) DeleteSecret(ctx context.Context, location secretmanagersvc.SecretLocation, metadata *secretmanagersvc.SecretMetadata) error {
	return c.DeleteSecretByName(ctx, location.OrgName, location.SecretRefName(), metadata)
}

// DeleteSecretByName removes the secret named secretRefName. Idempotent, and a secret managed by another owner is left alone.
func (c *Client) DeleteSecretByName(ctx context.Context, orgName, secretRefName string, metadata *secretmanagersvc.SecretMetadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	row, err := c.find(ctx, c.db, orgName, secretRefName, false)
	if errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		return nil // Idempotent - already deleted
	}
	if err != nil {
		return err
	}
	if row.ManagedBy != metadata.ManagedBy {
		return nil
	}
	if err := c.db.WithContext(ctx).Where("id = ?", row.ID).Delete(&secretRow{}).Error; err != nil {
		return fmt.Errorf("failed to delete secret %q: %w", secretRefName, err)
	}
	return nil
}

// GetSecret returns the secret's metadata without its values.
func (c *Client) GetSecret(ctx context.Context, location secretmanagersvc.SecretLocation) (*secretmanagersvc.SecretInfo, error) {
	name := location.SecretRefName()
	row, err := c.find(ctx, c.db, location.OrgName, name, false)
	if err != nil {
		return nil, err
	}
	return &secretmanagersvc.SecretInfo{
		ID:        name,
		Name:      name,
		Keys:      row.Keys,
		Labels:    row.Labels,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}

// GetSecretData returns all values of the secret named secretRefName.
func (c *Client) GetSecretData(ctx context.Context, orgName, secretRefName string) (map[string]string, error) {
	row, err := c.find(ctx, c.db, orgName, secretRefName, false)
	if err != nil {
		return nil, err
	}
	return c.open(ctx, row)
}

// Resolve returns the plaintext value of one key of a secret. It serves the
// in-house ClusterSecretStore only and must never be reachable from user APIs.
func (c *Client) Resolve(ctx context.Context, ouID, name, key string) (string, error) {
	row, err := c.find(ctx, c.db, ouID, name, false)
	if err != nil {
		return "", err
	}
	data, err := c.open(ctx, row)
	if err != nil {
		return "", err
	}
	value, ok := data[key]
	if !ok {
		return "", secretmanagersvc.ErrSecretNotFound
	}
	return value, nil
}

// Close releases nothing; the database handle is owned by the caller.
func (c *Client) Close(context.Context) error {
	return nil
}
