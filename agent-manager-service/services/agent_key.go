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

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// AgentKeyVerifier verifies the agent keys (agent tokens) agents present to
// module agent-key routes.
type AgentKeyVerifier interface {
	VerifyAgentKey(token string) (modkit.AgentKey, error)
}

var _ AgentKeyVerifier = (*agentTokenManagerService)(nil)

// VerifyAgentKey checks an agent token against the core's own signing keys
// and issuer, and returns the agent it names. Login tokens share the signing
// key, so the issuer and the agent claims are what tell them apart.
func (s *agentTokenManagerService) VerifyAgentKey(token string) (modkit.AgentKey, error) {
	var c AgentTokenClaims
	parsed, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if typ, _ := t.Header["typ"].(string); typ == WorkloadTokenType {
			return nil, errors.New("a workload token is not an agent key")
		}
		kid, _ := t.Header["kid"].(string)
		s.keyPairMutex.RLock()
		defer s.keyPairMutex.RUnlock()
		kp, ok := s.keyPairs[kid]
		if !ok {
			return nil, fmt.Errorf("unknown key %q", kid)
		}
		return kp.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithIssuer(s.config.Issuer),
		jwt.WithExpirationRequired())
	if err != nil {
		return modkit.AgentKey{}, err
	}
	if !parsed.Valid || c.OrgId == "" || c.ComponentUid == "" || c.ProjectUid == "" || c.EnvironmentUid == "" {
		return modkit.AgentKey{}, errors.New("malformed agent key")
	}
	agent := c.Agent
	if agent == "" {
		// Keys issued before the name claims name the agent only as subject.
		agent = c.Subject
	}
	return modkit.AgentKey{
		KeyID: c.ID, OrgID: c.OrgId, OrgName: c.Organization,
		ProjectID: c.ProjectUid, ProjectName: c.Project,
		AgentID: c.ComponentUid, AgentName: agent, EnvironmentID: c.EnvironmentUid,
		ExpiresAt: c.ExpiresAt.Time,
	}, nil
}
