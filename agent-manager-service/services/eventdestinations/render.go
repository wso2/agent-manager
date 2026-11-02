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
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/wso2/agent-manager/agent-manager-service/events"
)

// Text is an event rendered for people: a short title and a few lines of
// detail. Kinds that deliver to chat, email or paging services send this
// instead of the CloudEvent JSON.
type Text struct {
	Title string
	Body  string
}

// dataNameKeys are the event data fields that name what the event is about,
// in the order they are tried.
var dataNameKeys = []string{"displayName", "name", "agentName", "monitorName", "projectName"}

// Render turns the CloudEvent in msg into Text. It never fails: a body that is
// not a CloudEvent is rendered from msg's type alone.
func Render(msg Message) Text {
	var evt events.Event
	_ = json.Unmarshal(msg.Body, &evt)
	name := strings.TrimPrefix(msg.Type, events.TypePrefix)
	title := humanize(name)

	var lines []string
	data, _ := evt.Data.(map[string]any)
	if s, ok := data["message"].(string); ok && s != "" {
		lines = append(lines, s)
	} else if def, ok := events.Lookup(name); ok && def.Description != "" {
		lines = append(lines, def.Description)
	}
	for _, key := range dataNameKeys {
		if s, ok := data[key].(string); ok && s != "" {
			title += ": " + s
			break
		}
	}

	var where []string
	org := evt.OrgHandle
	if org == "" {
		org = evt.OrgID
	}
	for _, kv := range [][2]string{
		{"Organization", org}, {"Project", evt.Project}, {"Agent", evt.Agent}, {"Environment", evt.Environment},
	} {
		if kv[1] != "" {
			where = append(where, kv[0]+": "+kv[1])
		}
	}
	if len(where) > 0 {
		lines = append(lines, strings.Join(where, " · "))
	}
	if !evt.Time.IsZero() {
		lines = append(lines, "Time: "+evt.Time.UTC().Format(time.RFC1123))
	}
	lines = append(lines, fmt.Sprintf("Event: %s (%s)", name, msg.ID))
	return Text{Title: "Agent Manager · " + title, Body: strings.Join(lines, "\n")}
}

// humanize turns an event type such as "agent.build_failed" into
// "Agent build failed".
func humanize(name string) string {
	s := strings.NewReplacer(".", " ", "_", " ").Replace(name)
	if s == "" {
		return "Event"
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
