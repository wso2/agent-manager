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

package azure

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

func findingByName(t *testing.T, res *sa.Result, name string) *sa.Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.DisplayName == name {
			return f
		}
	}
	t.Fatalf("no finding named %q", name)
	return nil
}

func signal(f *sa.Finding, code string) *sa.Signal {
	for i := range f.Signals {
		if f.Signals[i].Code == code {
			return &f.Signals[i]
		}
	}
	return nil
}

func callResult(res *sa.Result, call string) *models.ShadowScanCallResult {
	for i := range res.Calls {
		if res.Calls[i].Call == call {
			return &res.Calls[i]
		}
	}
	return nil
}

func assertNoMarkers(t *testing.T, res any, logs string) {
	t.Helper()
	b, err := json.Marshal(res)
	require.NoError(t, err)
	for _, m := range retentionMarkers {
		assert.NotContains(t, string(b), m, "result must not contain %q", m)
		assert.NotContains(t, logs, m, "logs must not contain %q", m)
	}
}

func TestBuildURL(t *testing.T) {
	got := buildURL("https://arm.example.com/", "/subscriptions/a b/x", qp{"api-version", "1"}, qp{"$filter", "a eq 'b'"})
	assert.Equal(t, "https://arm.example.com/subscriptions/a%20b/x?api-version=1&$filter=a%20eq%20%27b%27", got)
}
