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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// Workload tokens are signed with the agent token key, so the JWKS endpoint
// already publishes what verifies them. They are told apart from agent and
// login tokens by their issuer, audience and type header: nothing that
// trusts agent tokens (such as the gateway) accepts one.
const (
	// WorkloadTokenIssuer is the iss of every workload token.
	WorkloadTokenIssuer = "agent-manager-service/workload"
	// WorkloadTokenType is the typ header of every workload token.
	WorkloadTokenType = "amp-workload+jwt"
	// workloadAudiencePrefix prefixes the module name in the aud claim.
	workloadAudiencePrefix = "amp-module:"
)

// WorkloadTokenSigner mints and verifies module workload tokens.
type WorkloadTokenSigner interface {
	IssueWorkload(w modkit.Workload, ttl time.Duration) (string, time.Time, error)
	VerifyWorkload(token string) (modkit.Workload, error)
}

var _ WorkloadTokenSigner = (*agentTokenManagerService)(nil)

type workloadClaims struct {
	jwt.RegisteredClaims
	Module string            `json:"mod"`
	OrgID  string            `json:"org_id,omitempty"`
	Scope  string            `json:"scope"`
	Extra  map[string]string `json:"ext,omitempty"`
}

func (s *agentTokenManagerService) IssueWorkload(w modkit.Workload, ttl time.Duration) (string, time.Time, error) {
	if w.Module == "" || w.Subject == "" || len(w.Scopes) == 0 {
		return "", time.Time{}, errors.New("a workload token needs a module, a subject and a scope")
	}
	now := time.Now()
	exp := now.Add(ttl)
	claims := workloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    WorkloadTokenIssuer,
			Subject:   w.Subject,
			Audience:  jwt.ClaimStrings{workloadAudiencePrefix + w.Module},
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Module: w.Module, OrgID: w.OrgID, Scope: strings.Join(w.Scopes, " "), Extra: w.Claims,
	}
	s.keyPairMutex.RLock()
	keyPair, ok := s.keyPairs[s.activeKeyID]
	s.keyPairMutex.RUnlock()
	if !ok {
		return "", time.Time{}, fmt.Errorf("active signing key not found: %s", s.activeKeyID)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyPair.KeyID
	token.Header["typ"] = WorkloadTokenType
	signed, err := token.SignedString(keyPair.PrivateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign workload token: %w", err)
	}
	return signed, exp, nil
}

func (s *agentTokenManagerService) VerifyWorkload(token string) (modkit.Workload, error) {
	var claims workloadClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if typ, _ := t.Header["typ"].(string); typ != WorkloadTokenType {
			return nil, errors.New("not a workload token")
		}
		kid, _ := t.Header["kid"].(string)
		s.keyPairMutex.RLock()
		defer s.keyPairMutex.RUnlock()
		kp, ok := s.keyPairs[kid]
		if !ok {
			return nil, fmt.Errorf("unknown key %q", kid)
		}
		return kp.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithIssuer(WorkloadTokenIssuer),
		jwt.WithExpirationRequired())
	if err != nil {
		return modkit.Workload{}, err
	}
	if !parsed.Valid || claims.Module == "" || claims.Subject == "" ||
		len(claims.Audience) != 1 || claims.Audience[0] != workloadAudiencePrefix+claims.Module {
		return modkit.Workload{}, errors.New("malformed workload token")
	}
	return modkit.Workload{
		Module: claims.Module, Subject: claims.Subject, OrgID: claims.OrgID,
		Scopes: strings.Fields(claims.Scope), Claims: claims.Extra,
	}, nil
}
