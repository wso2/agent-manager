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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type gitSecretStoreStub struct {
	GitSecretStore
	read func(context.Context, string, string) (map[string]string, error)
}

func (s gitSecretStoreStub) ReadGitSecret(ctx context.Context, ouID, name string) (map[string]string, error) {
	return s.read(ctx, ouID, name)
}

func TestGetGitCredentials(t *testing.T) {
	store := gitSecretStoreStub{read: func(_ context.Context, ouID, name string) (map[string]string, error) {
		require.Equal(t, "tenant", ouID)
		require.Equal(t, "source", name)
		return map[string]string{"username": "git", "password": "token"}, nil
	}}
	service := NewGitCredentialsService(store)
	got, err := service.GetGitCredentials(context.Background(), "tenant", "source")
	require.NoError(t, err)
	require.Equal(t, &GitCredentials{Type: GitSecretTypeBasic, Username: "git", Password: "token"}, got)

	_, err = service.GetGitCredentials(context.Background(), "tenant", "../source")
	require.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestGetGitCredentialsPreservesStoreErrors(t *testing.T) {
	for _, want := range []error{utils.ErrGitSecretNotFound, errors.New("database unavailable")} {
		store := gitSecretStoreStub{read: func(context.Context, string, string) (map[string]string, error) {
			return nil, want
		}}
		_, err := NewGitCredentialsService(store).GetGitCredentials(context.Background(), "tenant", "source")
		require.ErrorIs(t, err, want)
	}
}
