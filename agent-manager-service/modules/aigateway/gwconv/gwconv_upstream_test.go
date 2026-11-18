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

package gwconv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func upstreamStrPtr(s string) *string { return &s }

// The sandbox branch used to read its header from config.Main.Auth, so a provider
// with a sandbox credential and no main auth panicked on every read.
func TestConvertModelToSpecUpstream_SandboxAuthWithoutMainAuth(t *testing.T) {
	converted := ConvertModelToSpecUpstreamConfig(models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{URL: "https://api.example"},
		Sandbox: &models.UpstreamEndpoint{
			URL: "https://sandbox.example",
			Auth: &models.UpstreamAuth{
				Type:   upstreamStrPtr("api-key"),
				Header: upstreamStrPtr("X-Sandbox-Key"),
				Value:  upstreamStrPtr("sk-sandbox-secret"),
			},
		},
	})

	require.NotNil(t, converted.Sandbox)
	require.NotNil(t, converted.Sandbox.Auth)
	assert.Equal(t, "api-key", converted.Sandbox.Auth.Type)
	assert.Equal(t, "X-Sandbox-Key", *converted.Sandbox.Auth.Header,
		"the sandbox header must come from the sandbox endpoint, not from main")
	assert.Equal(t, "***REDACTED***", *converted.Sandbox.Auth.Value)
	assert.Nil(t, converted.Main.Auth)
}

// Type is a *string on the model and a plain string on the wire, so a decoded
// request that omitted it used to panic on the way back out.
func TestConvertModelToSpecUpstream_AuthWithoutType(t *testing.T) {
	converted := ConvertModelToSpecUpstreamConfig(models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{
			URL:  "https://api.example",
			Auth: &models.UpstreamAuth{Value: upstreamStrPtr("sk-secret")},
		},
	})

	require.NotNil(t, converted.Main.Auth)
	assert.Equal(t, "", converted.Main.Auth.Type)
	assert.Equal(t, "***REDACTED***", *converted.Main.Auth.Value)
}

func TestConvertModelToSpecUpstream_MasksBothEndpoints(t *testing.T) {
	converted := ConvertModelToSpecUpstreamConfig(models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{
			URL:  "https://api.example",
			Auth: &models.UpstreamAuth{Type: upstreamStrPtr("api-key"), Value: upstreamStrPtr("sk-main")},
		},
		Sandbox: &models.UpstreamEndpoint{
			URL:  "https://sandbox.example",
			Auth: &models.UpstreamAuth{Type: upstreamStrPtr("api-key"), Value: upstreamStrPtr("sk-sandbox")},
		},
	})

	assert.Equal(t, "***REDACTED***", *converted.Main.Auth.Value)
	assert.Equal(t, "***REDACTED***", *converted.Sandbox.Auth.Value)
}
