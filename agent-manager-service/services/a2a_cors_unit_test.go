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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestWithA2AVersionHeader(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"appended when missing", []string{"authorization", "Content-Type"}, []string{"authorization", "Content-Type", "A2A-Version"}},
		{"kept when present", []string{"A2A-Version"}, []string{"A2A-Version"}},
		{"kept when present in another case", []string{"a2a-version", "X-API-Key"}, []string{"a2a-version", "X-API-Key"}},
		{"added to an empty list", nil, []string{"A2A-Version"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := resolvedCORSConfig{CORSAllowHeaders: tc.in}
			got := withA2AVersionHeader(in)
			assert.Equal(t, tc.want, got.CORSAllowHeaders)
		})
	}
}

func TestWithA2AVersionHeaderDoesNotAliasInput(t *testing.T) {
	headers := make([]string, 1, 4)
	headers[0] = "Content-Type"
	withA2AVersionHeader(resolvedCORSConfig{CORSAllowHeaders: headers})
	assert.Equal(t, []string{"Content-Type"}, headers[:1])
	assert.Equal(t, "", headers[:2][1], "the caller's backing array is untouched")
}

func TestResolveCardCORSOverride(t *testing.T) {
	stored := &models.AgentConfig{}
	stored.SetCardCORSOverride(&models.CardCORS{Enabled: true, AllowOrigins: []string{"https://s.example"}})

	t.Run("absent request keeps stored", func(t *testing.T) {
		got := resolveCardCORSOverride(stored, nil)
		require.NotNil(t, got)
		assert.Equal(t, []string{"https://s.example"}, got.AllowOrigins)
	})
	t.Run("absent request and no row inherits", func(t *testing.T) {
		assert.Nil(t, resolveCardCORSOverride(nil, nil))
	})
	t.Run("inherit clears stored", func(t *testing.T) {
		assert.Nil(t, resolveCardCORSOverride(stored, &spec.AgentCardCORSConfig{Inherit: spec.PtrBool(true)}))
	})
	t.Run("explicit request replaces stored", func(t *testing.T) {
		got := resolveCardCORSOverride(stored, &spec.AgentCardCORSConfig{
			Enabled:          spec.PtrBool(true),
			AllowOrigin:      []string{"*"},
			AllowHeaders:     []string{"Content-Type"},
			AllowCredentials: spec.PtrBool(false),
		})
		assert.Equal(t, &models.CardCORS{Enabled: true, AllowOrigins: []string{"*"}, AllowHeaders: []string{"Content-Type"}}, got)
	})
}

func TestValidateCardCORS(t *testing.T) {
	enabledNoOrigins := &models.CardCORS{Enabled: true}
	wildcardCreds := &models.CardCORS{Enabled: true, AllowOrigins: []string{"*"}, AllowCredentials: true}
	ok := &models.CardCORS{Enabled: true, AllowOrigins: []string{"*"}}
	req := &spec.AgentCardCORSConfig{Enabled: spec.PtrBool(true)}

	assert.ErrorIs(t, validateCardCORS(false, req, ok), utils.ErrInvalidInput, "non-A2A agents cannot set it")
	assert.NoError(t, validateCardCORS(false, nil, nil), "and are untouched when they don't")
	assert.ErrorIs(t, validateCardCORS(true, &spec.AgentCardCORSConfig{}, nil), utils.ErrInvalidInput, "enabled is required unless inheriting")
	assert.NoError(t, validateCardCORS(true, &spec.AgentCardCORSConfig{Inherit: spec.PtrBool(true)}, nil))
	assert.ErrorIs(t, validateCardCORS(true, req, enabledNoOrigins), utils.ErrInvalidInput)
	assert.ErrorIs(t, validateCardCORS(true, req, wildcardCreds), utils.ErrInvalidInput)
	assert.NoError(t, validateCardCORS(true, req, ok))
	assert.NoError(t, validateCardCORS(true, nil, &models.CardCORS{Enabled: false}), "a disabled card needs no origins")
}
