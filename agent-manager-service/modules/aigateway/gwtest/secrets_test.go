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

	"github.com/stretchr/testify/require"
)

func TestStagingSecretsStoreOnlyStagedValues(t *testing.T) {
	ctx := context.Background()
	s := NewStagingSecrets()

	_, err := s.Prepare(ctx, "org", "p", "")
	require.Error(t, err, "an empty value cannot be staged")
	require.Error(t, s.Put(ctx, "org", "p", "v"), "an unstaged value is refused")

	_, err = s.Prepare(ctx, "org", "p", "v")
	require.NoError(t, err)
	require.Error(t, s.Put(ctx, "org", "p", "other"), "only the staged value is accepted")
	require.NoError(t, s.Put(ctx, "org", "p", "v"))
	got, err := s.Get(ctx, "org", "p")
	require.NoError(t, err)
	require.Equal(t, "v", got)
}
