//
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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

// The monitor evaluation template reads the org UUID off the run's labels to
// stamp it on evaluation pods; nothing else puts it on a monitor run.
func TestCreateWorkflowRun_StampsOrgUUIDLabel(t *testing.T) {
	var gotBody gen.WorkflowRun
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusCreated)
		require.NoError(t, json.NewEncoder(w).Encode(gotBody))
	}))

	const ouID = "019eafc8-0f23-7974-9119-d762c59c83a1"
	_, err := srv.CreateWorkflowRun(context.Background(), ouID, CreateWorkflowRunRequest{
		Name:         "monitor-abc",
		WorkflowName: "monitor-evaluation-workflow",
		Parameters:   map[string]interface{}{},
	})

	require.NoError(t, err)
	require.NotNil(t, gotBody.Metadata.Labels)
	assert.Equal(t, ouID, (*gotBody.Metadata.Labels)[string(LabelKeyOrgUUID)])
}
