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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// hashSuffixedName is "<prefix>-<16 hex>": the sanitized prefixSource cut to proxyNamePrefixMaxLen,
// then the first 8 bytes of sha256(raw). The hash carries the identity; the prefix is only for people.
func hashSuffixedName(prefixSource, raw string) string {
	hash := sha256.Sum256([]byte(raw))
	prefix := sanitizeForK8sName(prefixSource)
	if len(prefix) > proxyNamePrefixMaxLen {
		prefix = strings.TrimRight(prefix[:proxyNamePrefixMaxLen], "-")
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(hash[:8]))
}

// agentGatewayName is an agent's gateway artifact name (metadata.name) in one environment.
// Hashing the slash-separated handle keeps project "a-b" + agent "c" apart from "a" + "b-c".
func agentGatewayName(projectName, agentName, envUUID string) string {
	return hashSuffixedName(agentName, agentEnvAPIArtifactHandle(projectName, agentName, envUUID))
}

// agentGatewayContext is the context path an agent is served at on its environment's gateway.
func agentGatewayContext(agentName string) string {
	return "/" + strings.TrimPrefix(strings.TrimSpace(agentName), "/")
}

// agentGatewayURL is where gateway serves the agent: its public vhost plus the agent's context path.
func agentGatewayURL(gateway *models.Gateway, agentName string) string {
	return strings.TrimRight(buildPublicProxyURL(gateway, nil), "/") + agentGatewayContext(agentName)
}
