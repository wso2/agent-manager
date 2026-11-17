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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ocShadowPublishScope = "amp:shadow-agent:publish"

func TestOrgClientTokenMinter_MintsATokenForTheRequestedScope(t *testing.T) {
	claims := btClaims(btOrg, btClientID, time.Hour)
	claims["scope"] = ocShadowPublishScope
	token := btToken(t, claims)
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
	srv := idp.server(t)
	minter := newOrgClientTokenMinter(srv.URL+"/oauth2/token", btClientID, btClientSecret, "", srv.Client())

	got, expiresAt, err := minter.mint(context.Background(), btOrg, ocShadowPublishScope, 10*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, token, got)
	assert.WithinDuration(t, time.Unix(claims["exp"].(int64), 0), expiresAt, 0, "the expiry is the token's own")
	assert.Equal(t, "/ou/"+btOrg+"/oauth2/token", idp.path)
	assert.Equal(t, []string{ocShadowPublishScope}, idp.form["scope"], "only the requested scope is requested")
}

func TestOrgClientTokenMinter_RefusesATokenForAnotherScope(t *testing.T) {
	// The identity provider answers with a build token instead.
	token := btToken(t, btClaims(btOrg, btClientID, time.Hour))
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
	srv := idp.server(t)
	minter := newOrgClientTokenMinter(srv.URL+"/oauth2/token", btClientID, btClientSecret, "", srv.Client())

	got, expiresAt, err := minter.mint(context.Background(), btOrg, ocShadowPublishScope, 10*time.Minute)
	require.Error(t, err)
	assert.Empty(t, got)
	assert.True(t, expiresAt.IsZero())
	assert.Contains(t, err.Error(), "not only")
	assert.NotErrorIs(t, err, errBuildToken, "the minter's errors carry no build prefix")
	assert.NotContains(t, err.Error(), token, "the token must never appear in an error")
}
