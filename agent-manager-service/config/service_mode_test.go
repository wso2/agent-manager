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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModeFromArgs(t *testing.T) {
	assert.Equal(t, "dispatcher", modeFromArgs([]string{"-mode=dispatcher"}))
	assert.Equal(t, "api", modeFromArgs([]string{"-migrate", "--mode", "api"}))
	assert.Equal(t, "", modeFromArgs([]string{"-server=false"}))
	assert.Equal(t, "", modeFromArgs([]string{"mode=api"}), "a bare word is not a flag")
}

func TestValidateServiceMode(t *testing.T) {
	ok := func(c Config) { assert.Empty(t, validateServiceMode(&c)) }
	bad := func(c Config) { assert.NotEmpty(t, validateServiceMode(&c)) }

	ok(Config{Mode: ModeAll, Events: EventsConfig{Mode: EventsModeEmbedded}})
	ok(Config{Mode: ModeAPI, Events: EventsConfig{Mode: EventsModeOff}})
	ok(Config{
		Mode: ModeAPI, Events: EventsConfig{Mode: EventsModeNATS},
		Webhooks: WebhooksConfig{DispatcherURL: "http://d:8090", DispatcherAPIKey: "k"},
	})
	ok(Config{
		Mode: ModeDispatcher, Events: EventsConfig{Mode: EventsModeNATS},
		Webhooks: WebhooksConfig{DispatcherAPIKey: "k"},
	})

	bad(Config{Mode: ModeAPI, Events: EventsConfig{Mode: EventsModeNATS}})
	bad(Config{
		Mode: ModeAPI, Events: EventsConfig{Mode: EventsModeEmbedded},
		Webhooks: WebhooksConfig{DispatcherURL: "http://d:8090", DispatcherAPIKey: "k"},
	})
	bad(Config{
		Mode: ModeDispatcher, Events: EventsConfig{Mode: EventsModeEmbedded},
		Webhooks: WebhooksConfig{DispatcherAPIKey: "k"},
	})
	bad(Config{Mode: ModeDispatcher, Events: EventsConfig{Mode: EventsModeNATS}})
	bad(Config{Mode: "bogus"})
}
