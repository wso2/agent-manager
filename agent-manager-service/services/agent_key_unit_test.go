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
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// agentClaims are the claims of a valid agent key from the test issuer.
func agentClaims(exp time.Time) AgentTokenClaims {
	return AgentTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "agent-manager-test", Subject: "my-agent", ID: "jti-1",
			IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(exp),
		},
		ComponentUid: "comp-uuid", EnvironmentUid: "env-uuid", ProjectUid: "proj-uuid", OrgId: "org-123",
		Organization: "acme", Project: "proj", Agent: "my-agent",
	}
}

// sign signs claims as kid with key.
func sign(t *testing.T, claims jwt.Claims, kid string, key *rsa.PrivateKey) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func TestAgentTokenManager_VerifyAgentKey(t *testing.T) {
	svc := newTokenManager(t, fullClientMock("comp-uuid", "env-uuid", "proj-uuid"))
	verifier := svc.(AgentKeyVerifier)
	key := svc.(*agentTokenManagerService).keyPairs["key-1"].PrivateKey

	t.Run("a key the core issued names its agent", func(t *testing.T) {
		resp, err := svc.GenerateToken(auditableCtx(t), GenerateTokenRequest{
			OrgName: "acme", ProjectName: "proj", AgentName: "my-agent", OrgId: "org-123", ExpiresIn: "1h",
		})
		require.NoError(t, err)
		var c AgentTokenClaims
		_, _, err = jwt.NewParser().ParseUnverified(resp.Token, &c)
		require.NoError(t, err)
		got, err := verifier.VerifyAgentKey(resp.Token)
		require.NoError(t, err)
		assert.Equal(t, modkit.AgentKey{
			KeyID: c.ID, OrgID: "org-123", OrgName: "acme", ProjectID: "proj-uuid", ProjectName: c.Project,
			AgentID: "comp-uuid", AgentName: "my-agent", EnvironmentID: "env-uuid", ExpiresAt: c.ExpiresAt.Time,
		}, got)
	})

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	valid := sign(t, agentClaims(time.Now().Add(time.Hour)), "key-1", key)
	parts := strings.Split(valid, ".")
	tampered := agentClaims(time.Now().Add(time.Hour))
	tampered.OrgId = "org-other"
	tamperedPayload := strings.Split(sign(t, tampered, "key-1", other), ".")[1]

	login := agentClaims(time.Now().Add(time.Hour))
	login.Issuer = "amp-file-auth"
	noAgent := agentClaims(time.Now().Add(time.Hour))
	noAgent.ComponentUid = ""
	noExpiry := agentClaims(time.Now())
	noExpiry.ExpiresAt = nil
	workload, _, err := svc.(WorkloadTokenSigner).IssueWorkload(modkit.Workload{
		Module: "testmod", Subject: "run:1", OrgID: "org-123", Scopes: []string{"ping"},
	}, time.Minute)
	require.NoError(t, err)

	refused := map[string]string{
		"expired":             sign(t, agentClaims(time.Now().Add(-time.Minute)), "key-1", key),
		"another issuer":      sign(t, login, "key-1", key),
		"another signing key": sign(t, agentClaims(time.Now().Add(time.Hour)), "key-1", other),
		"an unknown kid":      sign(t, agentClaims(time.Now().Add(time.Hour)), "key-9", key),
		"a tampered payload":  parts[0] + "." + tamperedPayload + "." + parts[2],
		"no agent claim":      sign(t, noAgent, "key-1", key),
		"no expiry":           sign(t, noExpiry, "key-1", key),
		"a workload token":    workload,
		"not a token":         "not-a-jwt",
	}
	for name, token := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			_, err := verifier.VerifyAgentKey(token)
			assert.Error(t, err)
		})
	}
	got, err := verifier.VerifyAgentKey(valid)
	require.NoError(t, err, "the untampered key verifies")
	assert.Equal(t, "proj", got.ProjectName)

	t.Run("a key without the agent claim is named by its subject", func(t *testing.T) {
		old := agentClaims(time.Now().Add(time.Hour))
		old.Agent = ""
		got, err := verifier.VerifyAgentKey(sign(t, old, "key-1", key))
		require.NoError(t, err)
		assert.Equal(t, "my-agent", got.AgentName)
	})
}
