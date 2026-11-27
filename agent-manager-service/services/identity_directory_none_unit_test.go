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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
)

// With file sign-in there is no identity provider: every environment answers ErrNotProvisioned, and no agent has an
// identity provisioned.
func TestNoIdentityDirectory(t *testing.T) {
	dir := NoIdentityDirectory()

	client, err := dir.For(context.Background(), "00000000-0000-0000-0000-000000000001", "default")
	require.ErrorIs(t, err, identityv1.ErrNotProvisioned)
	assert.Nil(t, client)

	agents, err := dir.ListProvisionedAgents(context.Background(), "00000000-0000-0000-0000-000000000001", "default")
	require.NoError(t, err)
	assert.Empty(t, agents)
}
