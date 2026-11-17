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

package models

// PublishShadowScanRequest is what a source code scan command sends to the
// scan publisher endpoint: the code agents it confirmed and the repositories
// it scanned or could not scan.
type PublishShadowScanRequest struct {
	// Complete is true when the command could list every repository the
	// connection's settings name.
	Complete bool `json:"complete"`
	// ScannedRepositories are the repositories scanned to the end: a GitHub
	// repository name, or an Azure DevOps project/repository.
	ScannedRepositories []string `json:"scannedRepositories"`
	// FailedRepositories are the repositories the command could not scan.
	FailedRepositories []PublishShadowScanFailedRepository `json:"failedRepositories"`
	// Agents are the confirmed code agents only.
	Agents []PublishShadowScanAgent `json:"agents"`
}

// PublishShadowScanFailedRepository is a repository the command could not
// scan and why.
type PublishShadowScanFailedRepository struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// PublishShadowScanAgent is one confirmed code agent.
type PublishShadowScanAgent struct {
	// ExternalID is the command's stable ID of the agent.
	ExternalID string `json:"externalId"`
	Name       string `json:"name"`
	// Repository is the repository the agent is in, in the format of
	// ScannedRepositories.
	Repository string `json:"repository"`
	// Attributes go through the attribute allow-list; reviewedBy is required.
	Attributes map[string]any            `json:"attributes"`
	Evidence   []PublishShadowScanSignal `json:"evidence"`
}

// PublishShadowScanSignal is one piece of evidence: a D1 to D12 evidence
// code of the connection's provider, the file it was found in and what was
// observed (counts, flags and short labels only).
type PublishShadowScanSignal struct {
	Code     string         `json:"code"`
	Source   string         `json:"source"`
	Observed map[string]any `json:"observed"`
}

// PublishShadowScanResponse is the scan publisher endpoint's answer.
type PublishShadowScanResponse struct {
	ScanID        string `json:"scanId"`
	Status        string `json:"status"`
	FindingsCount int    `json:"findingsCount"`
}
