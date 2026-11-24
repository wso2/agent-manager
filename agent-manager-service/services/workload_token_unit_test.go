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
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func workloadSigner(t *testing.T) *agentTokenManagerService {
	t.Helper()
	return newTokenManager(t, &repomocks.PlatformRepositoryMock{}).(*agentTokenManagerService)
}

func TestWorkloadTokenRoundTrip(t *testing.T) {
	s := workloadSigner(t)
	w := modkit.Workload{
		Module: "aigateway", Name: "sync", Instance: "pod-1", Subject: "aigateway/sync", OrgID: "org-1",
		Scopes: []string{"amp:llm:read", "amp:llm:write"}, Claims: map[string]string{"gw": "g1"},
	}
	token, exp, err := s.IssueWorkload(w, time.Minute)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(time.Minute), exp, 5*time.Second)

	got, err := s.VerifyWorkload(token)
	require.NoError(t, err)
	assert.Equal(t, w, got)
}

func TestIssueWorkloadNeedsModuleSubjectAndScope(t *testing.T) {
	s := workloadSigner(t)
	for name, w := range map[string]modkit.Workload{
		"no module":  {Subject: "s", Scopes: []string{"x"}},
		"no subject": {Module: "m", Scopes: []string{"x"}},
		"no scope":   {Module: "m", Subject: "s"},
	} {
		_, _, err := s.IssueWorkload(w, time.Minute)
		require.Error(t, err, name)
	}

	broken := workloadSigner(t)
	broken.activeKeyID = "missing"
	_, _, err := broken.IssueWorkload(modkit.Workload{Module: "m", Subject: "s", Scopes: []string{"x"}}, time.Minute)
	require.ErrorContains(t, err, "active signing key not found")
}

// signRaw signs claims with the service's own key, so only the property under test makes the token invalid.
func signRaw(t *testing.T, s *agentTokenManagerService, claims jwt.Claims, typ, kid string) string {
	t.Helper()
	kp := s.keyPairs[s.activeKeyID]
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["typ"] = typ
	token.Header["kid"] = kid
	signed, err := token.SignedString(kp.PrivateKey)
	require.NoError(t, err)
	return signed
}

func TestVerifyWorkloadRejects(t *testing.T) {
	s := workloadSigner(t)
	valid := func() workloadClaims {
		now := time.Now()
		return workloadClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: WorkloadTokenIssuer, Subject: "m/s", Audience: jwt.ClaimStrings{"amp-module:m"},
				IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			},
			Module: "m", Scope: "x",
		}
	}
	kid := s.keyPairs[s.activeKeyID].KeyID
	require.NotEmpty(t, signRaw(t, s, valid(), WorkloadTokenType, kid))
	_, err := s.VerifyWorkload(signRaw(t, s, valid(), WorkloadTokenType, kid))
	require.NoError(t, err, "the baseline token is valid")

	expired := valid()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	noExpiry := valid()
	noExpiry.ExpiresAt = nil
	otherIssuer := valid()
	otherIssuer.Issuer = "agent-manager-service"
	otherModule := valid()
	otherModule.Audience = jwt.ClaimStrings{"amp-module:other"}
	twoAudiences := valid()
	twoAudiences.Audience = jwt.ClaimStrings{"amp-module:m", "gateway"}
	noModule := valid()
	noModule.Module = ""
	noSubject := valid()
	noSubject.Subject = ""

	for name, token := range map[string]string{
		"agent token type":    signRaw(t, s, valid(), "JWT", kid),
		"unknown key":         signRaw(t, s, valid(), WorkloadTokenType, "nope"),
		"expired":             signRaw(t, s, expired, WorkloadTokenType, kid),
		"no expiry":           signRaw(t, s, noExpiry, WorkloadTokenType, kid),
		"other issuer":        signRaw(t, s, otherIssuer, WorkloadTokenType, kid),
		"audience of another": signRaw(t, s, otherModule, WorkloadTokenType, kid),
		"two audiences":       signRaw(t, s, twoAudiences, WorkloadTokenType, kid),
		"no module":           signRaw(t, s, noModule, WorkloadTokenType, kid),
		"no subject":          signRaw(t, s, noSubject, WorkloadTokenType, kid),
		"garbage":             "not.a.token",
	} {
		_, err := s.VerifyWorkload(token)
		require.Error(t, err, name)
	}

	// A tampered payload fails the signature.
	good, _, err := s.IssueWorkload(modkit.Workload{Module: "m", Subject: "s", Scopes: []string{"x"}}, time.Minute)
	require.NoError(t, err)
	parts := strings.Split(good, ".")
	parts[1] = parts[1][:len(parts[1])-2] + "AA"
	_, err = s.VerifyWorkload(strings.Join(parts, "."))
	require.Error(t, err)
}
