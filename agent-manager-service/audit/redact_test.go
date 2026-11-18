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

package audit

import (
	"slices"
	"strings"
	"testing"
)

// TestUndeclaredDetailKeysAreDropped is the allow-list guarantee. A field that
// nobody declared must not reach a sink, however it got into the map — this is
// what makes the trail safe against a future emit site passing something it
// should not.
func TestUndeclaredDetailKeysAreDropped(t *testing.T) {
	e := Event{
		Action: ActionAuthzDeny,
		Details: map[string]any{
			"missingScope": "amp:agent:create",
			"password":     "hunter2",
			"clientSecret": "s3cr3t",
			"someNewField": "unvetted",
		},
	}

	redact(&e)

	if _, ok := e.Details["missingScope"]; !ok {
		t.Error("declared key missingScope was dropped")
	}
	for _, key := range []string{"password", "clientSecret", "someNewField"} {
		if _, ok := e.Details[key]; ok {
			t.Errorf("undeclared key %q survived redaction", key)
		}
	}

	dropped, ok := e.Details["_droppedKeys"].([]string)
	if !ok {
		t.Fatal("expected _droppedKeys to record what was removed")
	}
	for _, key := range []string{"clientSecret", "password", "someNewField"} {
		if !slices.Contains(dropped, key) {
			t.Errorf("_droppedKeys is missing %q; a schema gap should be visible", key)
		}
	}
}

func TestBaseFieldsAreAlwaysAllowed(t *testing.T) {
	e := Event{
		Action:  "agent:create",
		Details: map[string]any{"envelope": true, "attemptEventId": "abc"},
	}

	redact(&e)

	if len(e.Details) != 2 {
		t.Errorf("base fields should survive on any action, got %v", e.Details)
	}
}

// TestControlCharactersAreStripped covers log forging. Audit records carry
// attacker-controlled strings and are written to a log pipeline, so an injected
// newline could fabricate an entirely fake audit entry.
func TestControlCharactersAreStripped(t *testing.T) {
	e := Event{
		Action:       "agent:create",
		ResourceName: "evil\n{\"action\":\"agent:delete\",\"outcome\":\"success\"}",
		UserAgent:    "curl/8.0\r\nX-Injected: yes",
		ActorID:      "user\x00null",
	}

	redact(&e)

	for name, got := range map[string]string{
		"ResourceName": e.ResourceName,
		"UserAgent":    e.UserAgent,
		"ActorID":      e.ActorID,
	} {
		if strings.ContainsAny(got, "\n\r\x00") {
			t.Errorf("%s still contains control characters: %q", name, got)
		}
	}
}

func TestLongFieldsAreTruncated(t *testing.T) {
	e := Event{
		Action:    "agent:create",
		UserAgent: strings.Repeat("a", maxUserAgentLen*2),
	}

	redact(&e)

	if len([]rune(e.UserAgent)) > maxUserAgentLen+1 {
		t.Errorf("UserAgent was not truncated: %d runes", len([]rune(e.UserAgent)))
	}
}

// TestSecretShapedValuesAreMasked exercises the backstop. The primary control is
// that bodies are never read and Detail takes only scalars; this catches an emit
// site that passes credential material through a declared field anyway.
func TestSecretShapedValuesAreMasked(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"jwt", "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhbGljZSJ9", "[REDACTED:jwt]"},
		{"bearer", "Bearer abcdef0123456789abcdef", "[REDACTED:bearer]"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----", "[REDACTED:private-key]"},
		{"high entropy", strings.Repeat("A1b2C3d4", 6), "[REDACTED:high-entropy]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Event{
				Action:  ActionAuthzDeny,
				Details: map[string]any{"missingScope": tt.value},
			}
			redact(&e)

			got, _ := e.Details["missingScope"].(string)
			if !strings.Contains(got, tt.want) {
				t.Errorf("value %q was not masked: got %q, want it to contain %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestFingerprintIsNotReversible(t *testing.T) {
	secret := "sk-super-secret-api-key-value"
	fp := Fingerprint(secret)

	if strings.Contains(fp, "super-secret") {
		t.Errorf("fingerprint leaks the secret body: %q", fp)
	}
	if !strings.HasPrefix(fp, "sha256:") {
		t.Errorf("fingerprint %q should be labelled with its digest algorithm", fp)
	}
	if fp != Fingerprint(secret) {
		t.Error("fingerprint is not stable, so it cannot correlate two events")
	}
	if fp == Fingerprint(secret+"x") {
		t.Error("different secrets produced the same fingerprint")
	}
}

func TestFingerprintOfEmptyStringIsEmpty(t *testing.T) {
	if got := Fingerprint(""); got != "" {
		t.Errorf("Fingerprint(\"\") = %q, want empty", got)
	}
}

// TestAttributeKeySummaryNeverRecordsValues covers the free-form attribute map
// on user creation, which accepts passwords under arbitrary key names. The
// deny-list approach used elsewhere in this service filters only "password";
// recording keys alone cannot leak whatever the key happens to be called.
func TestAttributeKeySummaryNeverRecordsValues(t *testing.T) {
	attrs := map[string]string{
		"username":    "alice",
		"newPassword": "hunter2",
		"apiToken":    "sk-live-abc",
		"department":  "platform",
	}

	keys, count, containsSensitive := AttributeKeySummary(attrs)

	if count != len(attrs) {
		t.Errorf("count = %d, want %d", count, len(attrs))
	}
	if !containsSensitive {
		t.Error("containsSensitive should be true: newPassword and apiToken both match")
	}
	if !slices.IsSorted(keys) {
		t.Errorf("keys should be sorted for stable comparison across events: %v", keys)
	}

	joined := strings.Join(keys, ",")
	for _, secret := range []string{"hunter2", "sk-live-abc", "alice", "platform"} {
		if strings.Contains(joined, secret) {
			t.Errorf("attribute value %q leaked into the key summary", secret)
		}
	}
}

func TestAttributeKeySummaryPreservesLineSeparatorDistinctions(t *testing.T) {
	keys, _, _ := AttributeKeySummary(map[string]string{
		"team\u2028name": "",
		"team\u2029name": "",
		`team\u2028name`: "",
		"teamname":       "",
	})
	want := []string{`team\\u2028name`, `team\u2028name`, `team\u2029name`, "teamname"}
	if !slices.Equal(keys, want) {
		t.Errorf("keys = %q, want %q", keys, want)
	}
}

func TestAttributeKeySummaryWithoutSensitiveKeys(t *testing.T) {
	_, _, containsSensitive := AttributeKeySummary(map[string]string{"department": "platform"})
	if containsSensitive {
		t.Error("containsSensitive should be false when no key looks credential-shaped")
	}
}

func TestDetailRejectsNonScalarValues(t *testing.T) {
	type payload struct {
		Password string
	}

	e := Event{Action: ActionAuthzDeny, Details: map[string]any{}}
	Detail("missingScope", payload{Password: "hunter2"})(&e)

	got, _ := e.Details["missingScope"].(string)
	if strings.Contains(got, "hunter2") {
		t.Errorf("a struct value reached the record: %q", got)
	}
	if !strings.HasPrefix(got, "[unsupported:") {
		t.Errorf("expected an unsupported-type marker, got %q", got)
	}
}

func TestDetailKeysAreCapped(t *testing.T) {
	fields := map[string]FieldKind{}
	details := map[string]any{}
	for i := range maxDetailKeys * 2 {
		key := "field" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		fields[key] = KindName
		details[key] = "value"
	}
	RegisterDetailSchema("test:capped", fields)

	e := Event{Action: "test:capped", Details: details}
	redact(&e)

	// The cap plus the _droppedKeys marker the cap itself adds.
	if len(e.Details) > maxDetailKeys+1 {
		t.Errorf("details grew past the cap: %d keys", len(e.Details))
	}
}
