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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// TestResolveThunderURL covers the SINGLE centralized function every caller
// (EnvironmentService's SetThunderURL/GetThunderURL/ListThunderInstances, and
// the resolver's ReadThunderURLFunc via NewEnvThunderURLReader) delegates to.
func TestResolveThunderURL(t *testing.T) {
	t.Run("returns the registered on-prem record (handle + computed url)", func(t *testing.T) {
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(_ context.Context, ouID, envName string) (*models.EnvThunderURL, error) {
				assert.Equal(t, "ou-1", ouID)
				assert.Equal(t, "prod", envName)
				return &models.EnvThunderURL{ThunderHandle: strPtr("x7f2q9kz"), ThunderURL: "http://x7f2q9kz.amp.localhost:8080"}, nil
			},
		}

		rec, err := ResolveThunderURL(context.Background(), urlRepo, "ou-1", "prod")
		require.NoError(t, err)
		assert.Equal(t, "x7f2q9kz", rec.Handle)
		assert.Equal(t, "http://x7f2q9kz.amp.localhost:8080", rec.URL)
	})

	t.Run("returns the registered caller-supplied record (url only, no handle)", func(t *testing.T) {
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return &models.EnvThunderURL{ThunderURL: "https://stage-idp.tenant42.example.com"}, nil
			},
		}

		rec, err := ResolveThunderURL(context.Background(), urlRepo, "ou-1", "prod")
		require.NoError(t, err)
		assert.Empty(t, rec.Handle)
		assert.Equal(t, "https://stage-idp.tenant42.example.com", rec.URL)
	})

	t.Run("reports not-provisioned when no row exists — never recomputes a value", func(t *testing.T) {
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}

		rec, err := ResolveThunderURL(context.Background(), urlRepo, "ou-1", "prod")
		require.NoError(t, err)
		assert.Empty(t, rec.Handle)
		assert.Empty(t, rec.URL)
	})

	t.Run("propagates an unexpected repo error", func(t *testing.T) {
		boom := errors.New("db down")
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, boom
			},
		}

		_, err := ResolveThunderURL(context.Background(), urlRepo, "ou-1", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}
