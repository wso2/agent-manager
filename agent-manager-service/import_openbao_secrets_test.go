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

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenBaoImportManifest(t *testing.T) {
	entries, err := readOpenBaoImportManifest(strings.NewReader(`{"secrets":[{"organizationId":"tenant","secretName":"agent-dev-secrets"}]}`))
	require.NoError(t, err)
	require.Equal(t, []openBaoImportEntry{{OrganizationID: "tenant", SecretName: "agent-dev-secrets"}}, entries)

	for _, manifest := range []string{
		`{"secrets":[]}`,
		`{"secrets":[{"organizationId":"platform","secretName":"bootstrap"}]}`,
		`{"secrets":[{"organizationId":"tenant","secretName":"../../other"}]}`,
		`{"secrets":[{"organizationId":"tenant","secretName":"shared"},{"organizationId":"other","secretName":"shared"}]}`,
		`{"secrets":[{"organizationId":"tenant","secretName":"agent-dev-secrets","value":"not-allowed"}]}`,
	} {
		_, err := readOpenBaoImportManifest(strings.NewReader(manifest))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "not-allowed")
	}
}

func TestOpenBaoImportReportsPartialFailure(t *testing.T) {
	entries := []openBaoImportEntry{
		{OrganizationID: "tenant", SecretName: "missing"},
		{OrganizationID: "tenant", SecretName: "existing"},
		{OrganizationID: "tenant", SecretName: "new"},
	}
	readFailure := errors.New("legacy source unavailable")
	read := func(_ context.Context, _, name string) (map[string]string, error) {
		if name == "missing" {
			return nil, readFailure
		}
		return map[string]string{"key": "test-fixture"}, nil
	}
	write := func(_ context.Context, org, name string, data map[string]string) (bool, error) {
		require.Equal(t, "tenant", org)
		require.Equal(t, map[string]string{"key": "test-fixture"}, data)
		return name == "new", nil
	}
	imported, unchanged, err := importOpenBaoSecrets(context.Background(), entries, read, write)
	require.ErrorIs(t, err, readFailure)
	require.Equal(t, 1, imported)
	require.Equal(t, 1, unchanged)
}
