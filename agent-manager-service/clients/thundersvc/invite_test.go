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
package thundersvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inviteFlowServer plays Thunder's USER_ONBOARDING flow. Each step answers with a
// fresh challenge token; withOUStep adds the multi-OU selection step.
func inviteFlowServer(t *testing.T, withOUStep bool, final any, steps *[]map[string]any) IdentityClient {
	t.Helper()
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/flow/execute", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		*steps = append(*steps, body)
		action, _ := body["action"].(string)
		next := map[string]any{"executionId": "exec-1", "challengeToken": "ct-" + action}
		switch action {
		case "":
			assert.Equal(t, "USER_ONBOARDING", body["flowType"])
		case "action_usertype":
			if withOUStep {
				next["data"] = map[string]any{"actions": []map[string]any{{"ref": "action_ou_selection"}}}
			}
		case "action_share_manually":
			_ = json.NewEncoder(w).Encode(final)
			return
		}
		_ = json.NewEncoder(w).Encode(next)
	})
	t.Cleanup(srv.Close)
	return NewIdentityClient(srv.URL, "sys-client", "sys-secret")
}

func actions(steps []map[string]any) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		a, _ := s["action"].(string)
		out = append(out, a)
	}
	return out
}

func TestInviteUserSingleOU(t *testing.T) {
	var steps []map[string]any
	c := inviteFlowServer(t, false, map[string]any{"data": map[string]any{"additionalData": map[string]any{"inviteLink": "https://id/invite?c=1"}}}, &steps)

	link, err := c.InviteUser(context.Background(), "new@example.com", "ou-1")
	require.NoError(t, err)
	assert.Equal(t, "https://id/invite?c=1", link)
	assert.Equal(t, []string{"", "action_usertype", "action_invite_user", "action_submit_email", "action_share_manually"}, actions(steps))

	// Each step carries the execution id and the challenge token the previous step returned.
	for i := 1; i < len(steps); i++ {
		assert.Equal(t, "exec-1", steps[i]["executionId"])
		prev, _ := steps[i-1]["action"].(string)
		assert.Equal(t, "ct-"+prev, steps[i]["challengeToken"], "step %d", i)
	}
	assert.Equal(t, map[string]any{"email": "new@example.com"}, steps[3]["inputs"])
}

func TestInviteUserMultiOUSelectsTheOU(t *testing.T) {
	var steps []map[string]any
	c := inviteFlowServer(t, true, map[string]any{"inviteLink": "https://id/l"}, &steps)

	link, err := c.InviteUser(context.Background(), "x@example.com", "ou-42")
	require.NoError(t, err)
	assert.Equal(t, "https://id/l", link)
	require.Equal(t, []string{"", "action_usertype", "action_ou_selection", "action_invite_user", "action_submit_email", "action_share_manually"}, actions(steps))
	assert.Equal(t, map[string]any{"ouId": "ou-42"}, steps[2]["inputs"])
}

func TestInviteUserWithoutLinkFails(t *testing.T) {
	var steps []map[string]any
	c := inviteFlowServer(t, false, map[string]any{"flowStatus": "COMPLETE"}, &steps)
	_, err := c.InviteUser(context.Background(), "x@example.com", "ou")
	require.ErrorContains(t, err, "inviteLink not found")
}

func TestInviteUserStepFailure(t *testing.T) {
	calls := 0
	srv := newTestThunderServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 2 {
			http.Error(w, "flow broke", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"executionId":"e"}`))
	})
	defer srv.Close()
	_, err := NewIdentityClient(srv.URL, "c", "s").InviteUser(context.Background(), "x@example.com", "ou")
	require.ErrorContains(t, err, "submit type")
	require.ErrorContains(t, err, "flow broke")
	assert.Equal(t, 2, calls, "the flow stops at the failing step")

	srv2 := newTestThunderServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) })
	defer srv2.Close()
	_, err = NewIdentityClient(srv2.URL, "c", "s").InviteUser(context.Background(), "x@example.com", "ou")
	require.ErrorContains(t, err, "start flow decode")
}

func TestExtractInviteLink(t *testing.T) {
	cases := map[string]map[string]any{
		"top level":               {"invite_link": "L"},
		"additionalInfo":          {"additionalInfo": map[string]any{"link": "L"}},
		"wrapped":                 {"result": map[string]any{"invitationLink": "L"}},
		"wrapped additional data": {"output": map[string]any{"additionalData": map[string]any{"inviteLink": "L"}}},
	}
	for name, m := range cases {
		assert.Equal(t, "L", extractInviteLink(m), name)
	}
	assert.Empty(t, extractInviteLink(map[string]any{"inviteLink": "", "data": "not a map"}))
}

func TestGeneratedClientCredentials(t *testing.T) {
	id1, err := GenerateClientID()
	require.NoError(t, err)
	id2, err := GenerateClientID()
	require.NoError(t, err)
	assert.NotEqual(t, id1, id2)
	raw, err := base64.RawURLEncoding.DecodeString(id1)
	require.NoError(t, err, "client id is unpadded base64url")
	assert.Len(t, raw, 16)

	secret, err := GenerateClientSecret()
	require.NoError(t, err)
	assert.NotContains(t, secret, "=")
	assert.GreaterOrEqual(t, len(secret), 32)
}

func TestThunderNaming(t *testing.T) {
	assert.Equal(t, "amp-thunder-acme-dev", ThunderNamespace("Acme", "Dev"))
	assert.Equal(t, "amp-thunder-acme-dev-system-client", ThunderSystemClientSecretName("acme", "dev"))
	base := "http://amp-thunder-acme-dev-service.amp-thunder-acme-dev.svc.cluster.local:8090"
	assert.Equal(t, base+"/oauth2/jwks", ThunderJWKSURL("acme", "dev"))
	assert.Equal(t, base+"/oauth2/token", ThunderTokenURL("acme", "dev"))
	assert.Equal(t, "https://id.example/oauth2/token", ThunderExternalTokenURL("https://id.example"))
	assert.Equal(t, "https://id.example/oauth2/jwks", ThunderExternalJWKSURL("https://id.example"))

	long := ThunderReleaseName(strings.Repeat("o", 40), "production")
	assert.LessOrEqual(t, len(long), 53)
	assert.NotEqual(t, long, ThunderReleaseName(strings.Repeat("o", 40), "production2"), "truncated names stay distinct")
	assert.Panics(t, func() { ThunderReleaseName("", "dev") })

	assert.Equal(t, "amp-agent-acme-proj-my-agent", ProjectAgentThunderAppName("Acme", "proj", "my_agent"))
	longApp := ProjectAgentThunderAppName("o", "p", strings.Repeat("a", 120))
	assert.Len(t, longApp, 100)
	assert.Panics(t, func() { ProjectAgentThunderAppName("o", "", "a") })
}
