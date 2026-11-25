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
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Some journeys check what only the server sees: the audit trail, which amp-api
// writes to stdout, and module state, which the `amp` CLI inside the container
// changes. Both go through docker, against the stack's amp-api container
// (AMP_API_CONTAINER, else the one Compose labels amp-api). Specs that need
// them skip when there is no such container, e.g. against a remote deployment.

// APIContainer returns the amp-api container, or "" when docker can't find one.
func APIContainer() string {
	if c := os.Getenv("AMP_API_CONTAINER"); c != "" {
		return c
	}
	out, err := exec.Command("docker", "ps", "-q", "--filter", "label=com.docker.compose.service=amp-api").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

// RunAMP runs the amp CLI inside the amp-api container and returns its output.
func RunAMP(args ...string) (string, error) {
	container := APIContainer()
	if container == "" {
		return "", fmt.Errorf("no amp-api container")
	}
	out, err := exec.Command("docker", append([]string{"exec", container, "amp"}, args...)...).CombinedOutput()
	return string(out), err
}

// AuditRecord is one audit event as amp-api's stdout sink writes it.
type AuditRecord struct {
	Action       string         `json:"action"`
	Outcome      string         `json:"outcome"`
	ActorType    string         `json:"actorType"`
	ActorID      string         `json:"actorId"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	ResourceName string         `json:"resourceName"`
	ProjectName  string         `json:"projectName"`
	StatusCode   int            `json:"statusCode"`
	Details      map[string]any `json:"details"`
	// Raw is the whole record, for asserting that no secret appears anywhere.
	Raw string `json:"-"`
}

// AuditRecordsSince returns the audit records amp-api wrote since t.
func AuditRecordsSince(t time.Time) ([]AuditRecord, error) {
	container := APIContainer()
	if container == "" {
		return nil, fmt.Errorf("no amp-api container")
	}
	// docker logs --since has second precision; step back one so nothing is lost.
	since := t.Add(-time.Second).UTC().Format(time.RFC3339)
	out, err := exec.Command("docker", "logs", "--since", since, container).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker logs: %w: %s", err, out)
	}
	var records []AuditRecord
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, []byte(`"log_type":"audit"`)) {
			continue
		}
		var envelope struct {
			Event json.RawMessage `json:"event"`
		}
		if json.Unmarshal(line, &envelope) != nil || len(envelope.Event) == 0 {
			continue
		}
		var rec AuditRecord
		if json.Unmarshal(envelope.Event, &rec) != nil {
			continue
		}
		rec.Raw = string(envelope.Event)
		records = append(records, rec)
	}
	return records, sc.Err()
}
