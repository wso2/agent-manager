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

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEffectiveCardCORSInheritsAgentCORS(t *testing.T) {
	cfg := &AgentConfig{CORSEnabled: true, CORSAllowOrigins: []string{"https://a.example"}, CORSAllowCredentials: true}
	got := cfg.EffectiveCardCORS()
	assert.Equal(t, CardCORS{
		Inherited: true, Enabled: true,
		AllowOrigins: []string{"https://a.example"}, AllowHeaders: []string{"Content-Type"},
		AllowCredentials: true,
	}, got)
}

func TestEffectiveCardCORSInheritsDisabledAgentCORS(t *testing.T) {
	cfg := &AgentConfig{CORSEnabled: false, CORSAllowOrigins: []string{"*"}}
	got := cfg.EffectiveCardCORS()
	assert.True(t, got.Inherited)
	assert.False(t, got.Enabled)
}

func TestCardCORSOverrideRoundTrips(t *testing.T) {
	cfg := &AgentConfig{CORSEnabled: false}
	override := &CardCORS{Enabled: true, AllowOrigins: []string{"*"}, AllowHeaders: []string{"Content-Type", "X-Trace"}}
	cfg.SetCardCORSOverride(override)

	assert.Equal(t, override, cfg.CardCORSOverride())
	assert.Equal(t, *override, cfg.EffectiveCardCORS(), "an override ignores agent CORS")

	cfg.SetCardCORSOverride(nil)
	assert.Nil(t, cfg.CardCORSOverride())
	assert.Nil(t, cfg.CardCORSEnabled)
	assert.Nil(t, cfg.CardCORSAllowOrigins)
	assert.Nil(t, cfg.CardCORSAllowHeaders)
	assert.Nil(t, cfg.CardCORSAllowCredentials)
	assert.True(t, cfg.EffectiveCardCORS().Inherited)
}

func TestCardCORSOverrideOnNilConfigInherits(t *testing.T) {
	var cfg *AgentConfig
	assert.Nil(t, cfg.CardCORSOverride())
}
