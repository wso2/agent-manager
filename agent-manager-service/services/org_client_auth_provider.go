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
	"sync"

	ocauth "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/auth"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
)

// OrgClientAuthProviderFactory builds the token source of an organization's own
// OpenChoreo client from that organization's scheduler client credentials. A
// deployment whose OpenChoreo API needs more than a plain client_credentials
// grant (a resource indicator, a scope) supplies its own.
type OrgClientAuthProviderFactory func(tokenURL, clientID, clientSecret string) client.AuthProvider

// orgClientAuthProviderSetting is the process-wide choice made once at startup, as with
// SetMCPResourceServerProvider: the provisioner is built by wire, and reading the factory
// when a client is built keeps the construction order out of the picture.
var orgClientAuthProviderSetting struct {
	mu      sync.RWMutex
	factory OrgClientAuthProviderFactory
}

// SetOrgClientAuthProviderFactory chooses how organization clients get their tokens.
// nil restores the default plain client_credentials grant.
func SetOrgClientAuthProviderFactory(factory OrgClientAuthProviderFactory) {
	orgClientAuthProviderSetting.mu.Lock()
	defer orgClientAuthProviderSetting.mu.Unlock()
	orgClientAuthProviderSetting.factory = factory
}

func newOrgClientAuthProvider(tokenURL, clientID, clientSecret string) client.AuthProvider {
	orgClientAuthProviderSetting.mu.RLock()
	factory := orgClientAuthProviderSetting.factory
	orgClientAuthProviderSetting.mu.RUnlock()

	if factory == nil {
		return ocauth.NewAuthProvider(ocauth.Config{
			TokenURL:     tokenURL,
			ClientID:     clientID,
			ClientSecret: clientSecret,
		})
	}
	return factory(tokenURL, clientID, clientSecret)
}
