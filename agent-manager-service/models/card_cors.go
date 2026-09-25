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

// CardCORSMethods are the only methods the public Agent Card route serves.
var CardCORSMethods = []string{"GET", "OPTIONS"}

// DefaultCardCORSHeaders is what an inherited card allows. A card fetch is a
// plain GET, so nothing beyond Content-Type is needed.
var DefaultCardCORSHeaders = []string{"Content-Type"}

// CardCORS is the CORS the gateway applies to an A2A agent's public Agent Card
// route, which sits outside the agent-wide policy chain.
type CardCORS struct {
	Inherited        bool
	Enabled          bool
	AllowOrigins     []string
	AllowHeaders     []string
	AllowCredentials bool
}

// CardCORSOverride returns the stored card CORS, or nil when the card inherits.
func (c *AgentConfig) CardCORSOverride() *CardCORS {
	if c == nil || c.CardCORSEnabled == nil {
		return nil
	}
	allowCredentials := c.CardCORSAllowCredentials != nil && *c.CardCORSAllowCredentials
	return &CardCORS{
		Enabled:          *c.CardCORSEnabled,
		AllowOrigins:     c.CardCORSAllowOrigins,
		AllowHeaders:     c.CardCORSAllowHeaders,
		AllowCredentials: allowCredentials,
	}
}

// SetCardCORSOverride stores o as the card's CORS; nil returns the card to
// inheriting. The four columns are always written together.
func (c *AgentConfig) SetCardCORSOverride(o *CardCORS) {
	if o == nil {
		c.CardCORSEnabled, c.CardCORSAllowOrigins, c.CardCORSAllowHeaders, c.CardCORSAllowCredentials = nil, nil, nil, nil
		return
	}
	enabled, allowCredentials := o.Enabled, o.AllowCredentials
	c.CardCORSEnabled = &enabled
	c.CardCORSAllowOrigins = o.AllowOrigins
	c.CardCORSAllowHeaders = o.AllowHeaders
	c.CardCORSAllowCredentials = &allowCredentials
}

// EffectiveCardCORS is the card CORS that is actually published.
func (c *AgentConfig) EffectiveCardCORS() CardCORS {
	if override := c.CardCORSOverride(); override != nil {
		return *override
	}
	return CardCORS{
		Inherited:        true,
		Enabled:          c.CORSEnabled,
		AllowOrigins:     c.CORSAllowOrigins,
		AllowHeaders:     DefaultCardCORSHeaders,
		AllowCredentials: c.CORSAllowCredentials,
	}
}
