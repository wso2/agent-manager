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
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateEventsConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         EventsConfig
		errContains string
	}{
		{name: "off needs nothing", cfg: EventsConfig{Mode: EventsModeOff}},
		{name: "embedded with delivery settings", cfg: EventsConfig{Mode: EventsModeEmbedded, DeliveryMaxAttempts: 6, DeliveryRetentionDays: 14}},
		{name: "nats with a URL", cfg: EventsConfig{Mode: EventsModeNATS, NATSURL: "nats://nats:4222", DeliveryMaxAttempts: 6, DeliveryRetentionDays: 14}},
		{name: "nats without a URL", cfg: EventsConfig{Mode: EventsModeNATS, DeliveryMaxAttempts: 6, DeliveryRetentionDays: 14}, errContains: "EVENTS_NATS_URL is required"},
		{name: "unknown mode", cfg: EventsConfig{Mode: "kafka"}, errContains: "EVENTS_MODE must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateEventsConfig(tt.cfg)
			if tt.errContains == "" {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), tt.errContains) {
				t.Fatalf("expected one error containing %q, got %v", tt.errContains, errs)
			}
		})
	}
}

func TestLoadEnvs_EventsConfig(t *testing.T) {
	required := map[string]string{
		"OPEN_CHOREO_BASE_URL": "http://localhost/api/v1",
		"DB_HOST":              "localhost",
		"DB_USER":              "unit",
		"DB_PASSWORD":          "unit",
		"DB_NAME":              "unit",
	}
	for k, v := range required {
		t.Setenv(k, v)
	}
	for _, key := range []string{
		"EVENTS_MODE", "EVENTS_NATS_URL", "EVENTS_ALLOW_PRIVATE_DESTINATIONS",
		"EVENTS_DELIVERY_MAX_ATTEMPTS", "EVENTS_DELIVERY_RETENTION_DAYS",
	} {
		t.Setenv(key, "")
	}

	t.Run("defaults", func(t *testing.T) {
		t.Setenv("EVENTS_MODE", "")
		t.Setenv("EVENTS_NATS_URL", "")
		loadEnvs()
		got := config.Events
		if got.Mode != EventsModeOff || got.AllowPrivateDestinations ||
			got.DeliveryMaxAttempts != 6 || got.DeliveryRetentionDays != 14 {
			t.Errorf("unexpected defaults: %+v", got)
		}
	})

	t.Run("overrides", func(t *testing.T) {
		t.Setenv("EVENTS_NATS_URL", "nats://nats:4222")
		t.Setenv("EVENTS_ALLOW_PRIVATE_DESTINATIONS", "true")
		t.Setenv("EVENTS_DELIVERY_MAX_ATTEMPTS", "3")
		t.Setenv("EVENTS_DELIVERY_RETENTION_DAYS", "7")
		loadEnvs()
		got := config.Events
		if got.Mode != EventsModeNATS || !got.AllowPrivateDestinations ||
			got.DeliveryMaxAttempts != 3 || got.DeliveryRetentionDays != 7 {
			t.Errorf("unexpected config: %+v", got)
		}
	})
}

func TestValidateEventsConfigRejectsInvalidDeliverySettings(t *testing.T) {
	for _, mode := range []string{EventsModeEmbedded, EventsModeNATS} {
		for _, value := range []int{0, -1} {
			cfg := EventsConfig{Mode: mode, NATSURL: "nats://localhost:4222", DeliveryMaxAttempts: value, DeliveryRetentionDays: value}
			errs := validateEventsConfig(cfg)
			if len(errs) != 2 {
				t.Fatalf("mode %s value %d: expected both delivery errors, got %v", mode, value, errs)
			}
		}
	}
}

func TestEventsConfigJSONOmitsNATSToken(t *testing.T) {
	data, err := json.Marshal(EventsConfig{NATSToken: "test-bus-credential"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "NATSToken") || strings.Contains(string(data), "test-bus-credential") {
		t.Fatal("serialized event configuration exposes the NATS token")
	}
}
