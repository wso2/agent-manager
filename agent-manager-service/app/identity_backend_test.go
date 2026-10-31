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

package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type identityTargetResolverStub struct {
	thundersvc.EnvironmentIdentityTargetResolver
}
type cpIdentityBackendStub struct {
	thundersvc.CPEnvironmentIdentityBackend
}

type identityPromotionManagerStub struct {
	services.IdentityPromotionManager
}

func (identityTargetResolverStub) ResolveIdentityTarget(context.Context, string, string, string) (thundersvc.EnvironmentIdentityTarget, error) {
	return thundersvc.EnvironmentIdentityTarget{}, nil
}

func TestIdentityBackendConfiguration(t *testing.T) {
	targets := identityTargetResolverStub{}
	backend := cpIdentityBackendStub{}
	promotion := identityPromotionManagerStub{}
	require.NoError(t, validateIdentityBackendConfiguration("LEGACY_DIRECT", nil, nil, nil))
	require.NoError(t, validateIdentityBackendConfiguration("CP_MANAGED", targets, backend, nil))
	require.NoError(t, validateIdentityBackendConfiguration("CP_MANAGED", targets, backend, promotion))
	require.NoError(t, validateIdentityBackendConfiguration("MIXED", targets, backend, promotion))
	require.Error(t, validateIdentityBackendConfiguration("LEGACY_DIRECT", targets, backend, nil))
	require.Error(t, validateIdentityBackendConfiguration("LEGACY_DIRECT", nil, nil, promotion))
	require.Error(t, validateIdentityBackendConfiguration("CP_MANAGED", nil, backend, nil))
	require.Error(t, validateIdentityBackendConfiguration("MIXED", targets, nil, nil))
	require.Error(t, validateIdentityBackendConfiguration("UNKNOWN", targets, backend, nil))
}
