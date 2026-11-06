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

package eventdestinations

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/events"
)

func TestRender(t *testing.T) {
	evt := events.New("evt_1", "agent.created", events.ScopeProject, "org-1", "default", "",
		time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC))
	evt.OrgHandle = "acme"
	evt.Data = map[string]any{"name": "hr-agent"}
	body, err := json.Marshal(evt)
	require.NoError(t, err)

	text := Render(Message{ID: evt.ID, Type: evt.Type, Body: body})
	assert.Equal(t, "Agent Manager · Agent created: hr-agent", text.Title)
	assert.Contains(t, text.Body, "Organization: acme · Project: default")
	assert.Contains(t, text.Body, "Time: Fri, 16 Oct 2026 09:00:00 UTC")
	assert.Contains(t, text.Body, "Event: agent.created (evt_1)")
}

func TestRender_NotACloudEvent(t *testing.T) {
	text := Render(Message{ID: "evt_2", Type: events.TypePrefix + "monitor_run.failed", Body: []byte("x")})
	assert.Equal(t, "Agent Manager · Monitor run failed", text.Title)
	assert.Contains(t, text.Body, "Event: monitor_run.failed (evt_2)")
}

func TestRender_MonitorAlerts(t *testing.T) {
	for _, tc := range []struct{ kind, title, body string }{
		{"score_threshold_breached", "Quality score dropped", "Accuracy mean 0.4 is below 0.8."},
		{"score_threshold_recovered", "Quality score recovered", "Accuracy mean 0.9 is above 0.8 again."},
		{"monitor_run_failed", "Monitor run failed", "Evaluation could not complete: provider unavailable."},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			evt := events.New("source:alert:"+tc.kind, "monitor.alert."+tc.kind, events.ScopeAgent, "org-1", "default", "assistant", time.Time{})
			evt.Environment = "production"
			evt.Data = map[string]any{"title": tc.title, "body": tc.body, "monitor": map[string]any{"monitorName": "quality", "monitorDisplayName": "Quality checks", "runId": "run-123"}}
			body, err := json.Marshal(evt)
			require.NoError(t, err)
			text := Render(Message{ID: evt.ID, Type: evt.Type, Body: body})
			assert.Equal(t, "Agent Manager · "+tc.title, text.Title)
			assert.Contains(t, text.Body, tc.body)
			assert.Contains(t, text.Body, "Monitor: Quality checks\nRun: run-123")
			assert.Contains(t, text.Body, "Project: default · Agent: assistant · Environment: production")
			assert.Contains(t, text.Body, "Event: monitor.alert."+tc.kind+" ("+evt.ID+")")
		})
	}
}

func TestRender_MonitorAlertMissingOptionalFields(t *testing.T) {
	evt := events.New("evt-1", "monitor.alert.monitor_run_failed", events.ScopeAgent, "org-1", "default", "assistant", time.Time{})
	evt.Data = map[string]any{"monitor": map[string]any{"monitorName": "quality"}}
	body, err := json.Marshal(evt)
	require.NoError(t, err)
	text := Render(Message{ID: evt.ID, Type: evt.Type, Body: body})
	assert.Equal(t, "Agent Manager · Monitor alert monitor run failed", text.Title)
	assert.Contains(t, text.Body, "Monitor: quality")
	assert.NotContains(t, text.Body, "Run:")
}
