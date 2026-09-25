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

package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The a2a-gateway-route Trait schema declares upstreamPort and nothing else, so
// any other parameter would be rejected by OpenChoreo when the Component is
// written.
func TestBuildTraitA2AGatewayRouteCarriesOnlyTheUpstreamPort(t *testing.T) {
	c := &openChoreoClient{}
	trait, err := c.buildTrait(context.Background(), "default", "default", "trip-planner", TraitRequest{
		TraitKind: TraitKindTrait,
		TraitType: TraitA2AGatewayRoute,
		Opts:      []TraitOption{WithUpstreamPort(9099)},
	})
	require.NoError(t, err)

	assert.Equal(t, "a2a-gateway-route", trait.Name)
	assert.Equal(t, "trip-planner-a2a-gateway-route", trait.InstanceName)
	require.NotNil(t, trait.Parameters)
	assert.Equal(t, map[string]interface{}{"upstreamPort": int32(9099)}, *trait.Parameters)
}
