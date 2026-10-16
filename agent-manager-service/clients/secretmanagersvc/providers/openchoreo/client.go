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

package openchoreo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	secretmanagersvc "github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// labelKeyManagedBy is the label used for ownership tracking on the underlying
// SecretReference. Keys in the openchoreo.dev/ namespace are reserved by the
// API, so a plain key is used.
const labelKeyManagedBy = secretmanagersvc.LabelKeyManagedBy

// Client implements the secretmanagersvc.SecretsClient interface backed by the
// OpenChoreo secret management API. Secrets are addressed by the SecretReference
// name derived from the SecretLocation (see SecretLocation.SecretRefName), and
// the API stores the values in the configured target plane's secret store.
type Client struct {
	oc              occlient.OpenChoreoClient
	targetPlaneKind string
	targetPlaneName string
}

// Ensure Client implements the interface.
var _ secretmanagersvc.SecretsClient = &Client{}

func validateMetadata(metadata *secretmanagersvc.SecretMetadata) error {
	if metadata == nil {
		return fmt.Errorf("secret metadata is required")
	}
	if strings.TrimSpace(metadata.ManagedBy) == "" {
		return fmt.Errorf("secret metadata managedBy is required")
	}
	return nil
}

// isManagedBy reports whether the secret's labels mark it as managed by managedBy.
func isManagedBy(labels map[string]string, managedBy string) bool {
	return labels[labelKeyManagedBy] == managedBy
}

// userLabels returns the labels to send on create/update: ownership plus any
// existing user-set labels, with reserved openchoreo.dev/ keys stripped (the
// API rejects them; they are system-managed).
func userLabels(existing map[string]string, managedBy string) map[string]string {
	labels := make(map[string]string, len(existing)+1)
	for k, v := range existing {
		if strings.HasPrefix(k, "openchoreo.dev/") {
			continue
		}
		labels[k] = v
	}
	labels[labelKeyManagedBy] = managedBy
	return labels
}

// PushSecret writes a secret via the OpenChoreo API, replacing all existing data.
// Returns the secret name, which is also the underlying SecretReference name.
func (c *Client) PushSecret(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, metadata *secretmanagersvc.SecretMetadata) (string, error) {
	if err := validateMetadata(metadata); err != nil {
		return "", err
	}

	name := location.SecretRefName()
	existing, err := c.oc.GetSecret(ctx, location.OrgName, name)
	if err != nil && !errors.Is(err, utils.ErrNotFound) {
		return "", fmt.Errorf("failed to check secret existence: %w", err)
	}

	if err == nil {
		// Secret exists — verify ownership before replacing it
		if !isManagedBy(existing.Labels, metadata.ManagedBy) {
			return "", secretmanagersvc.ErrNotManaged
		}
		if _, err := c.oc.UpdateSecret(ctx, location.OrgName, name, occlient.UpdateSecretRequest{
			Data:   data,
			Labels: userLabels(existing.Labels, metadata.ManagedBy),
		}); err != nil {
			return "", fmt.Errorf("failed to update secret: %w", err)
		}
		return name, nil
	}

	if _, err := c.oc.CreateSecret(ctx, location.OrgName, occlient.CreateSecretRequest{
		Name:            name,
		Data:            data,
		Labels:          userLabels(metadata.Labels, metadata.ManagedBy),
		TargetPlaneKind: c.targetPlaneKind,
		TargetPlaneName: c.targetPlaneName,
	}); err != nil {
		// Handle race condition: another caller may have created it between Get and Create
		if errors.Is(err, utils.ErrConflict) {
			if _, updateErr := c.oc.UpdateSecret(ctx, location.OrgName, name, occlient.UpdateSecretRequest{
				Data:   data,
				Labels: userLabels(metadata.Labels, metadata.ManagedBy),
			}); updateErr != nil {
				// The name is taken, yet the secret API cannot find a secret to
				// update: the SecretReference was created outside this client.
				if errors.Is(updateErr, utils.ErrNotFound) {
					return "", fmt.Errorf("failed to update secret after create conflict: %w: %w",
						secretmanagersvc.ErrReferenceNameTaken, updateErr)
				}
				return "", fmt.Errorf("failed to update secret after create conflict: %w", updateErr)
			}
			return name, nil
		}
		return "", fmt.Errorf("failed to create secret: %w", err)
	}

	return name, nil
}

// PatchSecret merges data with an existing secret. Keys in data are
// added/updated, keys in keysToDelete are removed, omitted keys are preserved.
// Returns the secret name, which is also the underlying SecretReference name.
func (c *Client) PatchSecret(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, keysToDelete []string, metadata *secretmanagersvc.SecretMetadata) (string, error) {
	if err := validateMetadata(metadata); err != nil {
		return "", err
	}

	name := location.SecretRefName()
	existing, err := c.oc.GetSecret(ctx, location.OrgName, name)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return "", secretmanagersvc.ErrSecretNotFound
		}
		return "", fmt.Errorf("failed to read secret: %w", err)
	}

	if !isManagedBy(existing.Labels, metadata.ManagedBy) {
		return "", secretmanagersvc.ErrNotManaged
	}

	// Merge client-side: the OpenChoreo update replaces all data, so the merge
	// semantics are applied against the current state.
	merged := make(map[string]string, len(existing.Data)+len(data))
	for k, v := range existing.Data {
		merged[k] = string(v)
	}
	maps.Copy(merged, data)
	for _, k := range keysToDelete {
		delete(merged, k)
	}

	if _, err := c.oc.UpdateSecret(ctx, location.OrgName, name, occlient.UpdateSecretRequest{
		Data:   merged,
		Labels: userLabels(existing.Labels, metadata.ManagedBy),
	}); err != nil {
		return "", fmt.Errorf("failed to patch secret: %w", err)
	}

	return name, nil
}

// DeleteSecret removes a secret via the OpenChoreo API. Idempotent: returns
// nil if the secret doesn't exist. Only deletes secrets whose managed-by label
// matches the provided metadata.
func (c *Client) DeleteSecret(ctx context.Context, location secretmanagersvc.SecretLocation, metadata *secretmanagersvc.SecretMetadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}

	name := location.SecretRefName()
	existing, err := c.oc.GetSecret(ctx, location.OrgName, name)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil // Idempotent - already deleted
		}
		return fmt.Errorf("failed to check secret existence: %w", err)
	}

	if !isManagedBy(existing.Labels, metadata.ManagedBy) {
		return nil // Not managed by the specified owner, skip deletion
	}

	if err := c.oc.DeleteSecret(ctx, location.OrgName, name); err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// GetSecret retrieves secret metadata without values.
func (c *Client) GetSecret(ctx context.Context, location secretmanagersvc.SecretLocation) (*secretmanagersvc.SecretInfo, error) {
	name := location.SecretRefName()
	secret, err := c.oc.GetSecret(ctx, location.OrgName, name)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil, secretmanagersvc.ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}

	return &secretmanagersvc.SecretInfo{
		ID:        name,
		Name:      secret.Name,
		Keys:      keys,
		Labels:    secret.Labels,
		CreatedAt: secret.CreatedAt,
	}, nil
}

// Close cleans up resources.
func (c *Client) Close(ctx context.Context) error {
	// The OpenChoreo client doesn't require explicit cleanup
	return nil
}
