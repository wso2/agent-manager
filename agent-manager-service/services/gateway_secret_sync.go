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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The gateway's secret sync: on startup the gateway lists the secrets its deployments name with their values, on
// reconnect it lists them without values and fetches the value of each whose hash changed, and on a deploy event it
// fetches the value of each handle it does not have yet. AMS answers for the deployments assigned to the calling
// gateway only, in its organization.

// ErrGatewaySecretNotFound is a handle no deployment of the calling gateway names.
var ErrGatewaySecretNotFound = errors.New("secret not found")

// gatewaySecretStatusActive is the only status AMS reports; the gateway syncs ACTIVE secrets only.
const gatewaySecretStatusActive = "ACTIVE"

// gatewaySecretIDSpace derives the uuid a secret is reported under from its handle, so no database id leaves AMS.
var gatewaySecretIDSpace = uuid.MustParse("6f1c1f8e-1d1b-5c43-9a7e-2b8c7f0d4a10")

// GatewaySecretSyncItem is one secret as the gateway's sync reads it. Value is set only when values were asked for.
type GatewaySecretSyncItem struct {
	ID          string    `json:"uuid"`
	Handle      string    `json:"handle"`
	DisplayName string    `json:"name"`
	Type        string    `json:"type"`
	Provider    string    `json:"provider"`
	Status      string    `json:"status"`
	Hash        string    `json:"hash"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Value       *string   `json:"value,omitempty"`
}

// GatewaySecretSyncList is the response of GET /secrets.
type GatewaySecretSyncList struct {
	List  []GatewaySecretSyncItem `json:"list"`
	Count int                     `json:"count"`
}

// ListGatewaySecrets returns the secrets the deployments assigned to the gateway name, with their values when
// includeValues is set.
func (s *GatewayInternalAPIService) ListGatewaySecrets(
	ctx context.Context, ouID, gatewayID string, updatedAfter *time.Time, includeValues bool,
) (*GatewaySecretSyncList, error) {
	if s.gatewaySecrets == nil {
		return nil, errors.New("gateway secret sync is not configured")
	}
	refs, err := s.gatewaySecrets.ListForGateway(ctx, ouID, gatewayID, updatedAfter)
	if err != nil {
		return nil, err
	}
	list := &GatewaySecretSyncList{List: make([]GatewaySecretSyncItem, 0, len(refs))}
	for _, ref := range refs {
		item := GatewaySecretSyncItem{
			ID:          uuid.NewSHA1(gatewaySecretIDSpace, []byte(ref.Handle)).String(),
			Handle:      ref.Handle,
			DisplayName: ref.Handle,
			Type:        "generic",
			Provider:    "agent-manager",
			Status:      gatewaySecretStatusActive,
			Hash:        gatewaySecretHash(ref.Handle, ref.Version),
			CreatedAt:   ref.CreatedAt,
			UpdatedAt:   ref.UpdatedAt,
		}
		if includeValues {
			value, err := s.GetGatewaySecretValue(ctx, ouID, gatewayID, ref.Handle)
			if err != nil {
				return nil, fmt.Errorf("secret %s: %w", ref.Handle, err)
			}
			item.Value = &value
		}
		list.List = append(list.List, item)
	}
	list.Count = len(list.List)
	return list, nil
}

// GetGatewaySecretValue returns the plaintext of a secret a deployment assigned to the gateway names, or
// ErrGatewaySecretNotFound.
func (s *GatewayInternalAPIService) GetGatewaySecretValue(ctx context.Context, ouID, gatewayID, handle string) (string, error) {
	if s.gatewaySecrets == nil {
		return "", errors.New("gateway secret sync is not configured")
	}
	stored, found, err := s.gatewaySecrets.ReadForGateway(ctx, ouID, gatewayID, handle)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrGatewaySecretNotFound
	}
	// The store keeps an upstream credential in the encrypted form an artifact's secretRef carries.
	plaintext, err := utils.DecryptSecretRef(string(stored), s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret: %w", err)
	}
	return plaintext, nil
}

// gatewaySecretHash changes when the value a handle names changes. It is derived from the handle and the secret's
// version, never from the value.
func gatewaySecretHash(handle string, version int) string {
	sum := sha256.Sum256([]byte(handle + "\x00" + strconv.Itoa(version)))
	return "sha256:" + hex.EncodeToString(sum[:])
}
