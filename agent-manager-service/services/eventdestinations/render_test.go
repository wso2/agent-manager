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
