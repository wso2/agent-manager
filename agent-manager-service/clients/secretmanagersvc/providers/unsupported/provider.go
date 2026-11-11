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

// Package unsupported is the secret provider used when no secret store is
// configured. Reads report the secret as missing and writes fail with
// secretmanagersvc.ErrNotSupported.
package unsupported

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

// ProviderName is the name the provider is selected by.
const ProviderName = "unsupported"

type provider struct{}

// NewProvider returns a provider with no backing secret store.
func NewProvider() secretmanagersvc.Provider { return provider{} }

func (provider) NewClient(*secretmanagersvc.StoreConfig) (secretmanagersvc.SecretsClient, error) {
	return client{}, nil
}

func (provider) ValidateConfig(*secretmanagersvc.StoreConfig) error { return nil }

func (provider) Capabilities() secretmanagersvc.StoreCapabilities {
	return secretmanagersvc.StoreCapabilityReadOnly
}

type client struct{}

func (client) PushSecret(context.Context, secretmanagersvc.SecretLocation, map[string]string, *secretmanagersvc.SecretMetadata) (string, error) {
	return "", secretmanagersvc.ErrNotSupported
}

func (client) PatchSecret(context.Context, secretmanagersvc.SecretLocation, map[string]string, []string, *secretmanagersvc.SecretMetadata) (string, error) {
	return "", secretmanagersvc.ErrNotSupported
}

func (client) DeleteSecret(context.Context, secretmanagersvc.SecretLocation, *secretmanagersvc.SecretMetadata) error {
	return nil
}

func (client) DeleteSecretByName(context.Context, string, string, *secretmanagersvc.SecretMetadata) error {
	return nil
}

func (client) GetSecret(context.Context, secretmanagersvc.SecretLocation) (*secretmanagersvc.SecretInfo, error) {
	return nil, secretmanagersvc.ErrSecretNotFound
}

func (client) Close(context.Context) error { return nil }
