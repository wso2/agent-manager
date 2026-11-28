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

package gwsvc

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// secretReplacer returns what a deployment's YAML carries in place of the secretRef ref.
type secretReplacer func(ref string) (string, error)

// gatewaySecretStatus is the status the gateway syncs a secret in; it skips any other.
const gatewaySecretStatus = "ACTIVE"

// GatewaySecret is a secret an artifact deployed to the gateway refers to, as the gateway's secret sync reads it.
// Value is set only when the caller asked for values.
type GatewaySecret struct {
	UUID   string  `json:"uuid"`
	Handle string  `json:"handle"`
	Name   string  `json:"name"`
	Hash   string  `json:"hash"`
	Status string  `json:"status"`
	Value  *string `json:"value,omitempty"`
}

// validateSecretDeliveryProperty refuses a secretDelivery property that names no secret delivery.
func validateSecretDeliveryProperty(properties map[string]interface{}) error {
	raw, ok := properties[gwmodels.SecretDeliveryProperty]
	if !ok {
		return nil
	}
	if v, isString := raw.(string); isString && gwmodels.ValidSecretDelivery(v) {
		return nil
	}
	return httpx.NewInvalidInputError(
		fmt.Sprintf("Property %q must be %q or %q", gwmodels.SecretDeliveryProperty,
			gwmodels.SecretDeliveryInline, gwmodels.SecretDeliveryReference),
		"invalid secretDelivery property")
}

// gatewaySecretHandleKeyPath is where each organization's secret store keeps the key its handles are derived with.
const gatewaySecretHandleKeyPath = "gateway/secret-handle-key"

// gatewaySecretHandleKeyBytes is the length of a handle key.
const gatewaySecretHandleKeyBytes = 32

// gatewaySecretHandle names the secret at path, holding value, as the gateway stores it, under the organization's
// handle key. The handle changes with the value, so a gateway that cached the old one fetches the new one when the
// artifact is redeployed; the gateway never refetches a handle it already holds. hash is what the gateway compares
// on its reconnect sync. Both are keyed, so a handle seen in an artifact or a gateway log can't be used to check a
// guess at the value.
func gatewaySecretHandle(key []byte, path, value string) (handle, hash string) {
	version := keyedDigest(key, "amp-gateway-secret-value\x00"+path+"\x00"+value)
	return gatewaySecretHandlePrefix(key, path) + version[:16], version
}

// gatewaySecretHandlePrefix is the part of path's handle that does not depend on its value, so a handle can be
// matched to its path without reading any secret.
func gatewaySecretHandlePrefix(key []byte, path string) string {
	return "amp-" + keyedDigest(key, "amp-gateway-secret-id\x00"+path)[:24] + "-"
}

func keyedDigest(key []byte, message string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// gatewaySecretHandleKey returns the organization's handle key, creating it the first time one of its secrets is
// sent by reference. It is kept in the organization's secret store, with its credentials, and read once per request
// rather than cached, so replicas agree on it. It is created only if absent, so replicas creating it at the same
// moment all use the one that was stored first; a key one of them overwrote would leave handles no gateway can fetch.
func (s *GatewayInternalAPIService) gatewaySecretHandleKey(ctx context.Context, ouID string) ([]byte, error) {
	stored, err := s.secrets.Get(ctx, ouID, gatewaySecretHandleKeyPath)
	if errors.Is(err, modkit.ErrNotFound) {
		creator, ok := s.secrets.(modkit.SecretCreator)
		if !ok {
			return nil, errors.New("the secret store cannot create the gateway secret handle key atomically")
		}
		fresh := make([]byte, gatewaySecretHandleKeyBytes)
		if _, err := rand.Read(fresh); err != nil {
			return nil, fmt.Errorf("failed to generate the gateway secret handle key: %w", err)
		}
		stored, err = creator.Create(ctx, ouID, gatewaySecretHandleKeyPath, hex.EncodeToString(fresh))
		if err != nil {
			return nil, fmt.Errorf("failed to store the gateway secret handle key: %w", err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the gateway secret handle key: %w", err)
	}
	key, err := hex.DecodeString(stored)
	if err != nil || len(key) != gatewaySecretHandleKeyBytes {
		return nil, errors.New("the gateway secret handle key is malformed")
	}
	return key, nil
}

// secretPlaceholder is the template the gateway resolves from its secret store.
func secretPlaceholder(handle string) string {
	return `{{ secret "` + handle + `" }}`
}

// secretReplacerFor returns what replaces a secretRef in the deployments the gateway fetches: the plaintext, or a
// {{ secret }} placeholder for a gateway that takes secrets by reference. The gateway and the handle key are looked
// up on the first secretRef only, so a deployment without one costs no query.
func (s *GatewayInternalAPIService) secretReplacerFor(ctx context.Context, ouID, gatewayID string) secretReplacer {
	delivery := ""
	var key []byte
	return func(ref string) (string, error) {
		if delivery == "" {
			d, err := s.gatewaySecretDelivery(gatewayID)
			if err != nil {
				return "", err
			}
			delivery = d
		}
		plaintext, err := s.readSecretRef(ctx, ouID, ref)
		if err != nil {
			return "", err
		}
		if delivery != gwmodels.SecretDeliveryReference {
			return plaintext, nil
		}
		if key == nil {
			if key, err = s.gatewaySecretHandleKey(ctx, ouID); err != nil {
				return "", err
			}
		}
		handle, _ := gatewaySecretHandle(key, ref, plaintext)
		return secretPlaceholder(handle), nil
	}
}

// gatewaySecretDelivery is the gateway's secret delivery.
func (s *GatewayInternalAPIService) gatewaySecretDelivery(gatewayID string) (string, error) {
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return "", fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil {
		return "", gwerr.ErrGatewayNotFound
	}
	return gateway.SecretDelivery(), nil
}

// ListGatewaySecrets lists the secrets the artifacts deployed to the gateway refer to, with their values when
// includeValues is set. A gateway that takes secrets inline has none to sync.
func (s *GatewayInternalAPIService) ListGatewaySecrets(
	ctx context.Context, ouID, gatewayID string, includeValues bool,
) ([]GatewaySecret, error) {
	secrets, err := s.gatewaySecrets(ctx, ouID, gatewayID)
	if err != nil {
		return nil, err
	}
	out := make([]GatewaySecret, 0, len(secrets))
	for _, sec := range secrets {
		item := GatewaySecret{
			UUID:   uuid.NewSHA1(uuid.NameSpaceOID, []byte(sec.handle)).String(),
			Handle: sec.handle, Name: sec.handle, Hash: sec.hash, Status: gatewaySecretStatus,
		}
		if includeValues {
			value := sec.value
			item.Value = &value
		}
		out = append(out, item)
	}
	return out, nil
}

// GetGatewaySecretValue returns the value of the secret handle names, when an artifact deployed to the gateway
// refers to it. Any other handle, including one of another gateway or organization, is not found. Only the secret
// the handle names is read, so another artifact's missing or unreadable credential does not affect it.
func (s *GatewayInternalAPIService) GetGatewaySecretValue(ctx context.Context, ouID, gatewayID, handle string) (string, error) {
	refs, err := s.gatewaySecretRefs(ctx, ouID, gatewayID)
	if err != nil {
		return "", err
	}
	if len(refs) == 0 {
		return "", gwerr.ErrGatewaySecretNotFound
	}
	key, err := s.gatewaySecretHandleKey(ctx, ouID)
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		if !strings.HasPrefix(handle, gatewaySecretHandlePrefix(key, ref)) {
			continue
		}
		value, err := s.readSecretRef(ctx, ouID, ref)
		if errors.Is(err, modkit.ErrNotFound) {
			return "", gwerr.ErrGatewaySecretNotFound
		}
		if err != nil {
			return "", err
		}
		// A handle of an earlier value of this secret is not found: the gateway fetches the current one when the
		// artifact naming it is redeployed.
		if current, _ := gatewaySecretHandle(key, ref, value); current != handle {
			return "", gwerr.ErrGatewaySecretNotFound
		}
		return value, nil
	}
	return "", gwerr.ErrGatewaySecretNotFound
}

type gatewaySecret struct {
	handle, hash, value string
}

// gatewaySecrets reads the secrets the gateway's deployed artifacts refer to, in handle order. A reference whose
// secret is gone, such as a credential cleared after the artifact was deployed, is left out, so it does not stop
// the gateway restoring the others; any other failure to read one fails the list.
func (s *GatewayInternalAPIService) gatewaySecrets(ctx context.Context, ouID, gatewayID string) ([]gatewaySecret, error) {
	refs, err := s.gatewaySecretRefs(ctx, ouID, gatewayID)
	if err != nil || len(refs) == 0 {
		return nil, err
	}
	key, err := s.gatewaySecretHandleKey(ctx, ouID)
	if err != nil {
		return nil, err
	}
	out := make([]gatewaySecret, 0, len(refs))
	for _, ref := range refs {
		value, err := s.readSecretRef(ctx, ouID, ref)
		if errors.Is(err, modkit.ErrNotFound) {
			slog.WarnContext(ctx, "GatewayInternalAPIService: a deployed artifact refers to a secret that no longer exists",
				"gatewayID", gatewayID, "secretRef", ref)
			continue
		}
		if err != nil {
			return nil, err
		}
		handle, hash := gatewaySecretHandle(key, ref, value)
		out = append(out, gatewaySecret{handle: handle, hash: hash, value: value})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].handle < out[j].handle })
	return out, nil
}

// gatewaySecretRefs lists the secretRefs the gateway's deployed artifacts name, without reading any secret. They are
// found the way the deployments are rendered for the gateway, so the gateway is offered exactly the handles it is
// sent. A gateway that takes secrets inline has none.
func (s *GatewayInternalAPIService) gatewaySecretRefs(ctx context.Context, ouID, gatewayID string) ([]string, error) {
	delivery, err := s.gatewaySecretDelivery(gatewayID)
	if err != nil {
		return nil, err
	}
	if delivery != gwmodels.SecretDeliveryReference {
		return nil, nil
	}
	rows, err := s.deploymentRepo.ListForGatewaySync(ctx, ouID, gatewayID)
	if err != nil {
		return nil, err
	}
	var deploymentIDs []uuid.UUID
	for _, row := range rows {
		if row.State != gwmodels.GatewaySyncStateDeployed {
			continue
		}
		if id, err := uuid.Parse(row.DeploymentID); err == nil {
			deploymentIDs = append(deploymentIDs, id)
		}
	}
	if len(deploymentIDs) == 0 {
		return nil, nil
	}
	contents, err := s.deploymentRepo.ListContentForGateway(ctx, ouID, gatewayID, deploymentIDs)
	if err != nil {
		return nil, err
	}

	refs := map[string]struct{}{}
	collect := func(ref string) (string, error) {
		refs[ref] = struct{}{}
		return "", nil
	}
	for _, c := range contents {
		kind, _, ok := s.artifacts.GatewayArtifact(c.ArtifactRef)
		if !ok {
			continue
		}
		if _, err := replaceSecretsFor(kind, string(c.Content), collect); err != nil {
			return nil, fmt.Errorf("failed to read the secret references of deployment %s: %w", c.DeploymentID, err)
		}
	}

	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out, nil
}
