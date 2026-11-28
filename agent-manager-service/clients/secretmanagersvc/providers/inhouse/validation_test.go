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

package inhouse_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/inhouse"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/vault/vaulttest"
)

func TestProviderValidatesBeforeTouchingTheDatabase(t *testing.T) {
	p := inhouse.NewProvider()
	require.Equal(t, secretmanagersvc.StoreCapabilityReadWrite, p.Capabilities())
	require.Error(t, p.ValidateConfig(nil))

	c, err := p.NewClient(&secretmanagersvc.StoreConfig{
		DB:      &gorm.DB{},
		Secrets: repositories.NewSecretStore(vaulttest.Local(t)),
	})
	require.NoError(t, err)
	ctx := context.Background()

	bad := []secretmanagersvc.SecretLocation{
		{EntityName: "e"},
		{OrgName: "o"},
		{OrgName: "o", ProjectName: "a/b", EntityName: "e"},
	}
	for _, loc := range bad {
		_, err = c.PushSecret(ctx, loc, map[string]string{"k": "v"}, nil)
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
		_, err = c.PatchSecret(ctx, loc, map[string]string{"k": "v"}, nil, nil)
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
		require.ErrorIs(t, c.DeleteSecret(ctx, loc, nil), httpx.ErrInvalidInput)
		_, err = c.GetSecret(ctx, loc)
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
	}

	_, err = c.PushSecret(ctx, secretmanagersvc.SecretLocation{OrgName: "o", EntityName: "e"}, nil, nil)
	require.ErrorIs(t, err, httpx.ErrInvalidInput, "empty data is refused")
	require.ErrorIs(t, c.DeleteSecretByName(ctx, "o", " ", nil), httpx.ErrInvalidInput)
	require.NoError(t, c.Close(ctx))
}
