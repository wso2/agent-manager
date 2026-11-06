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

package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nicholas-fedor/shoutrrr/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// sampleURLs holds a valid URL of each offered service; the tokens in them
// are made up.
var sampleURLs = map[string]string{
	"slack":      "slack://hook:WNA3PBYV6-F20DUQND3RQ-Webc4MAvoacrpPakR8phF0zi@webhook",
	"teams":      "teams://?host=prod-00.westus.logic.azure.com:443/workflows/abc/triggers/manual/paths/invoke",
	"googlechat": "googlechat://chat.googleapis.com/v1/spaces/FOO/messages?key=bar&token=baz",
	"discord":    "discord://token@123456789",
	"telegram":   "telegram://110201543:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw@telegram?chats=@amp",
	"mattermost": "mattermost://chat.example.com/token123",
	"rocketchat": "rocketchat://chat.example.com/tokenA/tokenB/general",
	"zulip":      "zulip://bot%40example.com:key@chat.example.com/?stream=ops&topic=amp",
	"smtp":       "smtp://user:pass@smtp.example.com:587/?fromaddress=a@example.com&toaddresses=b@example.com",
	"pagerduty":  "pagerduty:///eb243592faa24ba2a5511afdf565c889",
	"opsgenie":   "opsgenie://api.opsgenie.com/eb243592-faa2-4ba2-a551q-1afdf565c889",
	"ntfy":       "ntfy://ntfy.sh/amp-alerts",
	"gotify":     "gotify://gotify.example.com/Aaa.bbb.ccc.ddd",
	"pushover":   "pushover://shoutrrr:apitoken@userkey/?devices=phone",
	"twilio":     "twilio://ACxxxx:authtoken@+15550001111/+15550002222",
}

// Every offered service must parse, and every one that sends over HTTP must
// take the driver's client: one that ignored it would bypass the SSRF checks.
func TestOfferedServices_UseTheDriversClient(t *testing.T) {
	d := New(true)
	require.Len(t, sampleURLs, len(offered), "every offered service has a sample URL")
	for _, s := range offered {
		raw, ok := sampleURLs[s.ID]
		require.True(t, ok, "sample URL for %s", s.ID)
		svc, err := d.locate(raw)
		require.NoError(t, err, s.ID)
		_, setter := svc.(types.HTTPClientSetter)
		assert.Equal(t, s.http, setter, "%s: HTTP services take the driver's client", s.ID)
		assert.True(t, strings.HasPrefix(s.DocsPath, docsGuide+"#"), s.ID)
	}
}

func TestNormalizeConfig(t *testing.T) {
	d := New(false)
	ctx := context.Background()

	cfg, err := d.NormalizeConfig(ctx, json.RawMessage(`{"service":" discord ","url":" discord://token@123456789 ","extra":1}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"service":"discord","url":"discord://token@123456789"}`, string(cfg))

	cfg, err = d.NormalizeConfig(ctx, json.RawMessage(`{"service":"slack"}`))
	require.NoError(t, err, "the URL may be left out to keep the current one")
	assert.JSONEq(t, `{"service":"slack"}`, string(cfg))

	for name, raw := range map[string]string{
		"unknown service":         `{"service":"mqtt","url":"mqtt://broker/topic"}`,
		"scheme of another":       `{"service":"slack","url":"discord://token@123456789"}`,
		"unparseable":             `{"service":"telegram","url":"telegram://nope"}`,
		"not an object":           `[]`,
		"smtp to a private host":  `{"service":"smtp","url":"smtp://u:p@127.0.0.1:25/?fromaddress=a@x.io&toaddresses=b@x.io"}`,
		"self-hosted on loopback": `{"service":"gotify","url":"gotify://localhost/Aaa.bbb"}`,
	} {
		_, err := d.NormalizeConfig(ctx, json.RawMessage(raw))
		assert.ErrorIs(t, err, utils.ErrInvalidInput, name)
	}

	_, err = New(true).NormalizeConfig(ctx, json.RawMessage(`{"service":"gotify","url":"gotify://localhost/Aaa.bbb"}`))
	assert.NoError(t, err, "private hosts are allowed for local development")
}

func TestSplitCredentialRedactAndOrigin(t *testing.T) {
	d := New(false)
	stored, cred := d.SplitCredential(json.RawMessage(`{"service":"slack","url":"slack://hook:a-b-c@webhook"}`))
	assert.JSONEq(t, `{"service":"slack"}`, string(stored))
	assert.Equal(t, "slack://hook:a-b-c@webhook", cred)

	_, cred = d.SplitCredential(json.RawMessage(`{"service":"slack"}`))
	assert.Empty(t, cred)

	assert.JSONEq(t, `{"service":"slack"}`, string(d.RedactConfig(json.RawMessage(`{"service":"slack","url":"slack://x"}`))))
	assert.Equal(t, "slack", d.Origin(json.RawMessage(`{"service":"slack"}`)))
	_, ok, err := d.NewSecret()
	require.NoError(t, err)
	assert.False(t, ok, "the URL is supplied, not generated")
}

func message(t *testing.T) eventdestinations.Message {
	t.Helper()
	evt := events.New("evt_1", "agent.deployed", events.ScopeAgent, "org-1", "default", "hr-agent",
		time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC))
	evt.Environment = "prod"
	body, err := json.Marshal(evt)
	require.NoError(t, err)
	return eventdestinations.Message{ID: evt.ID, Type: evt.Type, Body: body}
}

func ntfyServer(t *testing.T, status int, got *string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = string(b)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return "ntfy://" + strings.TrimPrefix(srv.URL, "http://") + "/amp-alerts?scheme=http"
}

func TestDeliver(t *testing.T) {
	ctx := context.Background()
	cfg := json.RawMessage(`{"service":"ntfy"}`)

	var got string
	url := ntfyServer(t, http.StatusOK, &got)
	res := New(true).Deliver(ctx, eventdestinations.Destination{Config: cfg, Secret: url}, message(t))
	require.NoError(t, res.Err)
	assert.Contains(t, got, "Agent deployed")
	assert.Contains(t, got, "Project: default · Agent: hr-agent · Environment: prod")

	url = ntfyServer(t, http.StatusInternalServerError, &got)
	res = New(true).Deliver(ctx, eventdestinations.Destination{Config: cfg, Secret: url}, message(t))
	require.Error(t, res.Err)
	assert.True(t, res.Retryable)
	assert.NotContains(t, res.Err.Error(), "amp-alerts", "the service URL is not leaked")

	res = New(false).Deliver(ctx, eventdestinations.Destination{Config: cfg, Secret: url}, message(t))
	require.Error(t, res.Err, "the SSRF-safe client refuses a loopback server")

	res = New(true).Deliver(ctx, eventdestinations.Destination{Config: cfg}, message(t))
	assert.Error(t, res.Err, "a destination without a URL cannot send")
}

type failedNotifyTransport struct{}

func (failedNotifyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.DeadlineExceeded
}

func TestTeamsCredentialScrubbing(t *testing.T) {
	d := New(true)
	d.client = &http.Client{Transport: failedNotifyTransport{}}
	raw := "teams://?host=" + url.QueryEscape("https://synthetic.logic.azure.com/workflows/test/triggers/manual/paths/invoke?sig=SYNTHETIC_SECRET")
	result := d.Deliver(context.Background(), eventdestinations.Destination{Config: json.RawMessage(`{"service":"teams"}`), Secret: raw}, message(t))
	require.Error(t, result.Err)
	assert.NotContains(t, result.Err.Error(), "SYNTHETIC_SECRET", "credentials must not enter delivery history")
}

func TestSlackCredentialScrubbing(t *testing.T) {
	d := New(true)
	d.client = &http.Client{Transport: failedNotifyTransport{}}
	raw := "slack://hook:WNA3PBYV6-F20DUQND3RQ-Webc4MAvoacrpPakR8phF0zi@webhook"
	result := d.Deliver(context.Background(), eventdestinations.Destination{Config: json.RawMessage(`{"service":"slack"}`), Secret: raw}, message(t))
	require.Error(t, result.Err)
	assert.NotContains(t, result.Err.Error(), "Webc4MAvoacrpPakR8phF0zi", "the token in Slack's rebuilt webhook URL must not enter delivery history")
	assert.Contains(t, result.Err.Error(), "deadline exceeded", "the cause is kept")
}
