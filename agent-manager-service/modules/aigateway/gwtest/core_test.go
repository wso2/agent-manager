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

package gwtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The stand-in core answers only what the AI Gateway's tests fake; a call it doesn't fake panics, naming
// the method, so a test that reaches it fails loudly instead of reading a zero value.
func TestCoreRefusesWhatItDoesNotFake(t *testing.T) {
	c := &Core{}
	ctx := context.Background()
	assert.PanicsWithValue(t, "gwtest.Core: GetOrg", func() { _, _ = c.GetOrg(ctx, "o") })
	assert.PanicsWithValue(t, "gwtest.Core: GetOrgByHandle", func() { _, _ = c.GetOrgByHandle(ctx, "h") })
	org, err := c.EnsureOrg(ctx, "o")
	assert.NoError(t, err)
	assert.Equal(t, "o", org.ID, "without EnsureOrgFunc, the org is the one asked for")
}
