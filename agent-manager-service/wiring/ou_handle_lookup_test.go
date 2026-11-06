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

package wiring

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
)

func TestProvideOUHandleLookup(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		GetOUHandleByIDFunc: func(_ context.Context, ouID string) (string, error) {
			switch ouID {
			case "found":
				return "org-b", nil
			case "missing":
				return "", &thundersvc.NotFoundError{Message: "no such ou"}
			default:
				return "", errors.New("thunder unavailable")
			}
		},
	}
	lookup := ProvideOUHandleLookup(client)

	handle, err := lookup(context.Background(), "found")
	require.NoError(t, err)
	assert.Equal(t, "org-b", handle)

	_, err = lookup(context.Background(), "missing")
	assert.ErrorIs(t, err, middleware.ErrOverrideOrgNotFound)

	_, err = lookup(context.Background(), "other")
	require.Error(t, err)
	assert.NotErrorIs(t, err, middleware.ErrOverrideOrgNotFound, "only a 404 means the org does not exist")
}
