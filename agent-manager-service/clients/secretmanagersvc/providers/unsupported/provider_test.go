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
package unsupported

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

// With no secret store, writes must fail loudly, while reads and deletes behave as if nothing is stored.
func TestUnsupportedProvider(t *testing.T) {
	ctx := context.Background()
	p := NewProvider()
	assert.Equal(t, secretmanagersvc.StoreCapabilityReadOnly, p.Capabilities())
	require.NoError(t, p.ValidateConfig(&secretmanagersvc.StoreConfig{}))

	c, err := p.NewClient(&secretmanagersvc.StoreConfig{})
	require.NoError(t, err)
	loc := secretmanagersvc.SecretLocation{OrgName: "o", EntityName: "e"}

	_, err = c.PushSecret(ctx, loc, map[string]string{"k": "v"}, nil)
	require.ErrorIs(t, err, secretmanagersvc.ErrNotSupported)
	_, err = c.PatchSecret(ctx, loc, map[string]string{"k": "v"}, nil, nil)
	require.ErrorIs(t, err, secretmanagersvc.ErrNotSupported)

	_, err = c.GetSecret(ctx, loc)
	require.ErrorIs(t, err, secretmanagersvc.ErrSecretNotFound)

	require.NoError(t, c.DeleteSecret(ctx, loc, nil))
	require.NoError(t, c.DeleteSecretByName(ctx, "o", "e-secrets", nil))
	require.NoError(t, c.Close(ctx))

	// Through the management client, a write surfaces ErrNotSupported to the caller.
	mgmt, err := secretmanagersvc.NewSecretManagementClient(&secretmanagersvc.StoreConfig{Provider: ProviderName}, p)
	require.NoError(t, err)
	_, err = mgmt.CreateSecret(ctx, loc, map[string]string{"k": "v"})
	require.ErrorIs(t, err, secretmanagersvc.ErrNotSupported)
}
