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

package framework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	. "github.com/onsi/gomega"
)

// Journey specs touch many resources whose response shapes are only partly
// asserted, so these helpers take a path and an expected status and hand back
// the decoded body. Operations with a stable, widely reused shape still belong
// in operations/<resource>.

// Projects returns a project API path; the organization comes from the token: Projects("/%s", p).
func Projects(format string, args ...any) string {
	return "/api/v1/projects" + fmt.Sprintf(format, args...)
}

// API returns a core API path; the organization comes from the token: API(cfg, "/events/destinations").
func API(_ *Config, format string, args ...any) string {
	return "/api/v1" + fmt.Sprintf(format, args...)
}

// GW returns an AI Gateway module path below the organization.
func GW(cfg *Config, format string, args ...any) string {
	return "/m/ai-gateway/orgs/" + cfg.DefaultOrg + fmt.Sprintf(format, args...)
}

// Send issues the request and returns its status and body. It fails only when
// the request cannot be sent, so callers can poll on the status.
func Send(g Gomega, c *AMPClient, method, path string, body any) (int, []byte) {
	resp, err := c.Do(method, path, body)
	g.Expect(err).NotTo(HaveOccurred(), "%s %s", method, path)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	g.Expect(err).NotTo(HaveOccurred(), "read %s %s", method, path)
	return resp.StatusCode, data
}

// Call issues the request, asserts the status and returns the body.
func Call(g Gomega, c *AMPClient, method, path string, body any, want int) []byte {
	status, data := Send(g, c, method, path, body)
	g.Expect(status).To(Equal(want), "%s %s: %s", method, path, string(data))
	return data
}

// CallJSON issues the request, asserts the status and decodes the body into T.
func CallJSON[T any](g Gomega, c *AMPClient, method, path string, body any, want int) T {
	var out T
	data := Call(g, c, method, path, body, want)
	g.Expect(json.Unmarshal(data, &out)).To(Succeed(), "decode %s %s: %s", method, path, string(data))
	return out
}

// Obj is a decoded JSON object, for responses only a few fields of which matter.
type Obj = map[string]any

// CallObj issues the request, asserts the status and decodes a JSON object.
func CallObj(g Gomega, c *AMPClient, method, path string, body any, want int) Obj {
	return CallJSON[Obj](g, c, method, path, body, want)
}

// CallError issues the request, asserts an error status and returns the error
// envelope. The envelope must carry a message, so a bare status from a proxy or
// a panic doesn't pass for the handler's own refusal.
func CallError(g Gomega, c *AMPClient, method, path string, body any, want int) ErrorResponse {
	e := CallJSON[ErrorResponse](g, c, method, path, body, want)
	g.Expect(e.Message).NotTo(BeEmpty(), "%s %s: error response without a message", method, path)
	return e
}

// Str reads a string field from a decoded object, following nested keys.
func Str(o Obj, keys ...string) string {
	var cur any = o
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[k]
	}
	s, _ := cur.(string)
	return s
}

// Items reads a list field from a decoded object as objects.
func Items(o Obj, key string) []Obj {
	raw, _ := o[key].([]any)
	out := make([]Obj, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Field collects one string field from each object, for membership assertions.
func Field(items []Obj, key string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, Str(it, key))
	}
	return out
}

// GWAgent returns an AI Gateway path below an agent, with its project in the query: GWAgent(p, a, "/model-configs/%s", id).
func GWAgent(project, agent, format string, args ...any) string {
	p := "/m/ai-gateway/agents/" + url.PathEscape(agent) + fmt.Sprintf(format, args...)
	sep := "?"
	if strings.Contains(p, "?") {
		sep = "&"
	}
	return p + sep + url.Values{"project": {project}}.Encode()
}
