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

package cliagent

import (
	"encoding/json"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// MetricPoint is one time-series sample (matches the server's MetricDataPoint).
type MetricPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// Metrics is the data shape of `amctl agent metrics --json`.
type Metrics struct {
	CPUUsage []MetricPoint `json:"cpuUsage"`
	Memory   []MetricPoint `json:"memory"`
}

// LogEntry matches the server's runtime LogEntry.
type LogEntry struct {
	Log       string `json:"log"`
	LogLevel  string `json:"logLevel"`
	Timestamp string `json:"timestamp"`
}

// Logs is the data shape of `amctl agent logs --json`.
type Logs struct {
	Logs []LogEntry `json:"logs"`
}

// Traces is the data shape of `amctl agent traces --json`. We only assert on
// presence, so individual traces stay raw.
type Traces struct {
	Traces     []json.RawMessage `json:"traces"`
	TotalCount int               `json:"totalCount"`
}

// AgentMetrics runs `amctl agent metrics <name> --env <env>`.
func AgentMetrics(g Gomega, h *amctl.Harness, org, project, name, env string) Metrics {
	return amctl.DecodeData[Metrics](g, h.Run("agent", "metrics", name, "--org", org, "--project", project, "--json"))
}

// AgentLogs runs `amctl agent logs <name> --env <env>`.
func AgentLogs(g Gomega, h *amctl.Harness, org, project, name, env string) Logs {
	return amctl.DecodeData[Logs](g, h.Run("agent", "logs", name, "--org", org, "--project", project, "--json"))
}

// AgentTraces runs `amctl agent traces <name> --env <env>`.
func AgentTraces(g Gomega, h *amctl.Harness, org, project, name, env string) Traces {
	return amctl.DecodeData[Traces](g, h.Run("agent", "traces", name, "--org", org, "--project", project, "--json"))
}
