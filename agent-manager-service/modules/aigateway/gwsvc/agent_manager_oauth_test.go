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

package gwsvc

import (
	"errors"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// policyByName returns the policy map with the given name from a policy slice, or nil.
func policyByName(policies []map[string]interface{}, name string) map[string]interface{} {
	for _, p := range policies {
		if p["name"] == name {
			return p
		}
	}
	return nil
}

func policyParams(p map[string]interface{}) map[string]interface{} {
	if p == nil {
		return nil
	}
	params, _ := p["params"].(map[string]interface{})
	return params
}

func TestValidateAuthExclusivity(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  bool
		oauth   bool
		wantErr bool
	}{
		{"none", false, false, false},
		{"api-key only", true, false, false},
		{"oauth only", false, true, false},
		{"both rejected", true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAuthExclusivity(resolvedCORSConfig{
				EnableApiKeySecurity: tc.apiKey,
				EnableOAuthSecurity:  tc.oauth,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, utils.ErrInvalidInput) {
					t.Fatalf("expected ErrInvalidInput, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestResolveAPIConfig_OAuthDefaults(t *testing.T) {
	// withDefaults, no request, no DB: API key on, OAuth off, header defaults set.
	cfg := resolveAPIConfig(nil, nil, nil, nil, nil, true)
	if !cfg.EnableApiKeySecurity {
		t.Errorf("expected API key security on by default")
	}
	if cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth security off by default")
	}
	if cfg.OAuthHeaderName != models.DefaultOAuthHeaderName {
		t.Errorf("expected header name %q, got %q", models.DefaultOAuthHeaderName, cfg.OAuthHeaderName)
	}
	if cfg.OAuthAuthHeaderPrefix != models.DefaultOAuthAuthHeaderPrefix {
		t.Errorf("expected header prefix %q, got %q", models.DefaultOAuthAuthHeaderPrefix, cfg.OAuthAuthHeaderPrefix)
	}
	if !cfg.OAuthForwardToken {
		t.Errorf("expected forwardToken to default to true")
	}
}

func TestResolveAPIConfig_OAuthFromRequest(t *testing.T) {
	cfg := resolveAPIConfig(nil, boolPtr(false), nil, boolPtr(true), &gwspec.OAuthConfig{
		Issuers:          []string{"CustomKeyManager"},
		Audiences:        []string{"my-api"},
		HeaderName:       strPtr("X-Token"),
		AuthHeaderPrefix: strPtr("Token"),
		ForwardToken:     boolPtr(false),
	}, true)

	if cfg.EnableApiKeySecurity {
		t.Errorf("expected API key off")
	}
	if !cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth on")
	}
	if len(cfg.OAuthIssuers) != 1 || cfg.OAuthIssuers[0] != "CustomKeyManager" {
		t.Errorf("issuers not resolved from request: %v", cfg.OAuthIssuers)
	}
	if len(cfg.OAuthAudiences) != 1 || cfg.OAuthAudiences[0] != "my-api" {
		t.Errorf("audiences not resolved: %v", cfg.OAuthAudiences)
	}
	if cfg.OAuthHeaderName != "X-Token" || cfg.OAuthAuthHeaderPrefix != "Token" {
		t.Errorf("header overrides not resolved: %q / %q", cfg.OAuthHeaderName, cfg.OAuthAuthHeaderPrefix)
	}
	if cfg.OAuthForwardToken {
		t.Errorf("expected forwardToken override to false")
	}
}

func TestResolveAPIConfig_OAuthFromDB(t *testing.T) {
	existing := &gwmodels.AgentConfig{
		EnableApiKeySecurity:  false,
		EnableOAuthSecurity:   true,
		OAuthIssuers:          []string{"DBKeyManager"},
		OAuthHeaderName:       "Authorization",
		OAuthAuthHeaderPrefix: "Bearer",
	}
	// No request fields → resolve from DB.
	cfg := resolveAPIConfig(existing, nil, nil, nil, nil, false)
	if !cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth on from DB")
	}
	if len(cfg.OAuthIssuers) != 1 || cfg.OAuthIssuers[0] != "DBKeyManager" {
		t.Errorf("issuers not resolved from DB: %v", cfg.OAuthIssuers)
	}

	// Request overrides DB.
	cfg = resolveAPIConfig(existing, boolPtr(true), nil, boolPtr(false), nil, false)
	if cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth disabled by request override")
	}
	if !cfg.EnableApiKeySecurity {
		t.Errorf("expected API key enabled by request override")
	}
}

func TestBuildPolicies_Modes(t *testing.T) {
	base := resolvedCORSConfig{
		CORSEnabled:           true,
		CORSAllowOrigins:      []string{"*"},
		CORSAllowMethods:      []string{"GET"},
		CORSAllowHeaders:      []string{"Authorization"},
		OAuthHeaderName:       models.DefaultOAuthHeaderName,
		OAuthAuthHeaderPrefix: models.DefaultOAuthAuthHeaderPrefix,
	}

	t.Run("no auth", func(t *testing.T) {
		p := buildPolicies(base)
		if policyByName(p, "cors") == nil {
			t.Errorf("expected cors policy")
		}
		if policyByName(p, "api-key-auth") != nil || policyByName(p, "jwt-auth") != nil {
			t.Errorf("expected no auth policies, got %v", p)
		}
	})

	t.Run("no auth and no cors returns empty non-nil slice", func(t *testing.T) {
		// Must be non-nil so it marshals to [] not null — the api-configuration
		// trait rejects a null policies field.
		p := buildPolicies(resolvedCORSConfig{
			OAuthHeaderName:       models.DefaultOAuthHeaderName,
			OAuthAuthHeaderPrefix: models.DefaultOAuthAuthHeaderPrefix,
		})
		if p == nil {
			t.Fatalf("expected non-nil empty slice, got nil")
		}
		if len(p) != 0 {
			t.Errorf("expected 0 policies, got %v", p)
		}
	})

	t.Run("api key", func(t *testing.T) {
		cfg := base
		cfg.EnableApiKeySecurity = true
		p := buildPolicies(cfg)
		if policyByName(p, "api-key-auth") == nil {
			t.Errorf("expected api-key-auth policy")
		}
		if policyByName(p, "jwt-auth") != nil {
			t.Errorf("did not expect jwt-auth policy")
		}
	})

	t.Run("oauth with issuers and audiences", func(t *testing.T) {
		cfg := base
		cfg.EnableOAuthSecurity = true
		cfg.OAuthIssuers = []string{"MyKeyManager"}
		cfg.OAuthAudiences = []string{"my-api"}
		cfg.OAuthForwardToken = true
		p := buildPolicies(cfg)
		jwt := policyByName(p, "jwt-auth")
		if jwt == nil {
			t.Fatalf("expected jwt-auth policy")
		}
		if policyByName(p, "api-key-auth") != nil {
			t.Errorf("did not expect api-key-auth policy")
		}
		params := policyParams(jwt)
		issuers, _ := params["issuers"].([]string)
		if len(issuers) != 1 || issuers[0] != "MyKeyManager" {
			t.Errorf("expected issuers [MyKeyManager], got %v", params["issuers"])
		}
		audiences, _ := params["audiences"].([]string)
		if len(audiences) != 1 || audiences[0] != "my-api" {
			t.Errorf("expected audiences [my-api], got %v", params["audiences"])
		}
		// requiredClaims/requiredScopes are authorization params deferred to a
		// future authorization policy — they must never appear in jwt-auth.
		if params["requiredClaims"] != nil || params["requiredScopes"] != nil {
			t.Errorf("did not expect requiredClaims/requiredScopes in params, got %v", params)
		}
		if params["forwardToken"] != true {
			t.Errorf("expected forwardToken true, got %v", params["forwardToken"])
		}
		if params["headerName"] != models.DefaultOAuthHeaderName || params["authHeaderPrefix"] != models.DefaultOAuthAuthHeaderPrefix {
			t.Errorf("expected default header params, got %v / %v", params["headerName"], params["authHeaderPrefix"])
		}
	})

	t.Run("oauth empty issuers carries no default issuer", func(t *testing.T) {
		// No silent fallback: the issuers slice is passed through as-is. Empty
		// issuers are rejected upstream by validateOAuthSecurityConfig, so build
		// time must not invent a default.
		cfg := base
		cfg.EnableOAuthSecurity = true
		cfg.OAuthIssuers = nil
		p := buildPolicies(cfg)
		jwt := policyByName(p, "jwt-auth")
		if jwt == nil {
			t.Fatalf("expected jwt-auth policy")
		}
		issuers, _ := policyParams(jwt)["issuers"].([]string)
		if len(issuers) != 0 {
			t.Errorf("expected no issuers, got %v", issuers)
		}
	})
}

func TestValidateOAuthSecurityConfig(t *testing.T) {
	tests := []struct {
		name    string
		oauth   bool
		issuers []string
		wantErr bool
	}{
		{"oauth off, no issuers", false, nil, false},
		{"oauth on with issuer", true, []string{"ThunderKeyManager"}, false},
		{"oauth on, empty issuers rejected", true, nil, true},
		{"oauth on, empty slice rejected", true, []string{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOAuthSecurityConfig(resolvedCORSConfig{
				EnableOAuthSecurity: tc.oauth,
				OAuthIssuers:        tc.issuers,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, utils.ErrInvalidInput) {
					t.Fatalf("expected ErrInvalidInput, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
