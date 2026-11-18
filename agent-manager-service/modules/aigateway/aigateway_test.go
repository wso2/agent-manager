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

package aigateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

type fakeApplier struct{}

func (fakeApplier) ApplyIdentityProvider(context.Context, string, string, gwmodels.GatewayIdentityProvider) error {
	return nil
}

func (fakeApplier) DeleteIdentityProvider(context.Context, string, string, string) error { return nil }

// The zero options are the open-source behaviour; a distribution's Configure
// replaces them before the gateway is built.
func TestConfigureSetsOptions(t *testing.T) {
	t.Cleanup(func() { options.Store(nil) })
	require.Equal(t, Options{}, currentOptions())
	Configure(Options{GatewayConfigApplier: fakeApplier{}})
	require.Equal(t, fakeApplier{}, currentOptions().GatewayConfigApplier)
}
