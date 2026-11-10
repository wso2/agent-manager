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
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// GitSecretStore owns tenant-scoped Git credentials and their workflow references.
type GitSecretStore interface {
	CreateGitSecret(ctx context.Context, ouID, name, username, password string) error
	ListGitSecretNames(ctx context.Context, ouID string, limit, offset int) ([]string, int, error)
	CheckGitSecret(ctx context.Context, ouID, name string) error
	ReadGitSecret(ctx context.Context, ouID, name string) (map[string]string, error)
	DeleteGitSecret(ctx context.Context, ouID, name string) error
}

type GitCredentials struct {
	Type     string
	Username string
	Password string
}

type GitCredentialsService interface {
	GetGitCredentials(ctx context.Context, ouID, secretRef string) (*GitCredentials, error)
}

type gitCredentialsService struct {
	store GitSecretStore
}

func NewGitCredentialsService(store GitSecretStore) GitCredentialsService {
	return &gitCredentialsService{store: store}
}

func (s *gitCredentialsService) GetGitCredentials(ctx context.Context, ouID, secretRef string) (*GitCredentials, error) {
	if !isValidSecretName(secretRef) {
		return nil, fmt.Errorf("%w: invalid Git secret name", utils.ErrInvalidInput)
	}
	data, err := s.store.ReadGitSecret(ctx, ouID, secretRef)
	if err != nil {
		return nil, fmt.Errorf("read Git credentials: %w", err)
	}
	if data["password"] == "" {
		return nil, utils.ErrGitSecretInvalidType
	}
	return &GitCredentials{
		Type: GitSecretTypeBasic, Username: data["username"], Password: data["password"],
	}, nil
}
