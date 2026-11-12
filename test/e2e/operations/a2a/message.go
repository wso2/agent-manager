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

package a2a

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/gomega"

	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
)

const jsonRPCBinding = "JSONRPC"

// JSONRPCURL returns the url of the card's JSON-RPC supported interface, or ""
// when the card has none.
func JSONRPCURL(card json.RawMessage) string {
	var parsed struct {
		SupportedInterfaces []struct {
			URL             string `json:"url"`
			ProtocolBinding string `json:"protocolBinding"`
		} `json:"supportedInterfaces"`
	}
	if err := json.Unmarshal(card, &parsed); err != nil {
		return ""
	}
	for _, iface := range parsed.SupportedInterfaces {
		if iface.ProtocolBinding == jsonRPCBinding {
			return iface.URL
		}
	}
	return ""
}

// SentTask is the task the agent created for a SendMessage.
type SentTask struct {
	ID     string `json:"id"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

// SendMessage posts a JSON-RPC SendMessage to rpcURL and returns the task the
// agent created for it. Any final state passes, so the check does not depend
// on the agent's model call succeeding. An empty apiKey sends no X-API-Key header.
func SendMessage(rpcURL, apiKey, text string) SentTask {
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "SendMessage",
		"params": map[string]any{
			"message": map[string]any{
				"messageId": fmt.Sprintf("e2e-msg-%d", time.Now().UnixNano()),
				"role":      "ROLE_USER",
				"parts":     []map[string]string{{"text": text}},
			},
		},
	}
	headers := map[string]string{"A2A-Version": "1.0"}
	if apiKey != "" {
		headers["X-API-Key"] = apiKey
	}

	raw := agentops.InvokeEndpointWithHeaders(rpcURL, body, headers, true)

	var reply struct {
		Result *struct {
			Task *SentTask `json:"task"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	Expect(json.Unmarshal([]byte(raw), &reply)).To(Succeed(), "SendMessage reply is not JSON: %s", raw)
	Expect(reply.Error).To(BeEmpty(), "SendMessage reply: %s", raw)
	Expect(reply.Result).NotTo(BeNil(), "SendMessage reply has no result: %s", raw)
	Expect(reply.Result.Task).NotTo(BeNil(), "SendMessage result has no task: %s", raw)
	Expect(reply.Result.Task.ID).NotTo(BeEmpty(), "SendMessage task has no id: %s", raw)
	Expect(reply.Result.Task.Status.State).To(HavePrefix("TASK_STATE_"), "SendMessage task state: %s", raw)
	return *reply.Result.Task
}
