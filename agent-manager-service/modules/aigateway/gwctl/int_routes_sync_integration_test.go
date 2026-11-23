//go:build integration

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

package gwctl_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// intSyncFixture is a gateway holding an LLM provider, an agent and a REST
// API (a kind the gateway doesn't sync), and another gateway of the
// organization holding a fourth artifact.
type intSyncFixture struct {
	*intAPI
	provider, agent, rest, elsewhere             uuid.UUID
	providerDep, agentDep, restDep, elsewhereDep uuid.UUID
}

func newINTSyncFixture(t *testing.T) intSyncFixture {
	t.Helper()
	a := newINTAPI(t)
	f := intSyncFixture{intAPI: a}
	providerHandle, agentHandle, restHandle := unique("provider"), unique("agent"), unique("rest")
	f.provider = a.intArtifact(gwmodels.KindLLMProvider, providerHandle)
	f.providerDep = a.intDeploy(a.gateway.UUID, f.provider, gwmodels.KindLLMProvider, providerHandle,
		"kind: LlmProvider\nspec:\n  upstream:\n    url: https://llm.example.com\n")
	f.agent = a.intArtifact(gwmodels.KindAgent, agentHandle)
	f.agentDep = a.intDeploy(a.gateway.UUID, f.agent, gwmodels.KindAgent, agentHandle, "kind: Agent\nname: "+agentHandle+"\n")
	f.rest = a.intArtifact(gwmodels.KindRestAPI, restHandle)
	f.restDep = a.intDeploy(a.gateway.UUID, f.rest, gwmodels.KindRestAPI, restHandle, "kind: RestApi\n")
	other := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleBoth)
	otherHandle := unique("other")
	f.elsewhere = a.intArtifact(gwmodels.KindAgent, otherHandle)
	f.elsewhereDep = a.intDeploy(other.UUID, f.elsewhere, gwmodels.KindAgent, otherHandle, "kind: Agent\n")
	return f
}

// intDeploymentsResponse is the body of GET /deployments.
type intDeploymentsResponse struct {
	Deployments []struct {
		ArtifactID   string    `json:"artifactId"`
		DeploymentID string    `json:"deploymentId"`
		Kind         string    `json:"kind"`
		State        string    `json:"state"`
		DeployedAt   time.Time `json:"deployedAt"`
		Etag         string    `json:"etag"`
	} `json:"deployments"`
}

// TestINTRoutesDeploymentsListWhatTheGatewayShouldHold pins the startup-sync
// list: the calling gateway's deployments only, deployed or undeployed, each
// with a deterministic etag.
func TestINTRoutesDeploymentsListWhatTheGatewayShouldHold(t *testing.T) {
	f := newINTSyncFixture(t)

	list := expectInto[intDeploymentsResponse](t, f.call(http.MethodGet, "/deployments", nil), http.StatusOK)
	require.Len(t, list.Deployments, 2, "neither another gateway's deployment nor an unsynced kind is listed")
	byArtifact := map[string]int{}
	for i, d := range list.Deployments {
		byArtifact[d.ArtifactID] = i
		assert.Equal(t, gwmodels.GatewaySyncStateDeployed, d.State)
		assert.NotEmpty(t, d.Etag)
		assert.WithinDuration(t, time.Now(), d.DeployedAt, time.Minute)
	}
	require.Contains(t, byArtifact, f.provider.String())
	require.Contains(t, byArtifact, f.agent.String())
	provider := list.Deployments[byArtifact[f.provider.String()]]
	assert.Equal(t, f.providerDep.String(), provider.DeploymentID)
	assert.Equal(t, gwmodels.KindLLMProvider, provider.Kind)
	assert.Equal(t, gwmodels.KindAgent, list.Deployments[byArtifact[f.agent.String()]].Kind)

	again := expectInto[intDeploymentsResponse](t, f.call(http.MethodGet, "/deployments", nil), http.StatusOK)
	for _, d := range again.Deployments {
		if d.ArtifactID == f.provider.String() {
			assert.Equal(t, provider.Etag, d.Etag, "the etag is stable across calls")
		}
	}

	// An undeploy lists the artifact as undeployed, with no deployment id.
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		_, err := gwrepo.NewDeploymentRepo(f.db).BeginUndeploy(context.Background(), tx, gwmodels.Placement{
			ArtifactRef: gwmodels.ArtifactRef{OUID: f.org, ArtifactUUID: f.provider, Kind: gwmodels.KindLLMProvider},
			GatewayUUID: f.gateway.UUID,
		})
		return err
	}))
	list = expectInto[intDeploymentsResponse](t, f.call(http.MethodGet, "/deployments", nil), http.StatusOK)
	for _, d := range list.Deployments {
		if d.ArtifactID == f.provider.String() {
			assert.Equal(t, gwmodels.GatewaySyncStateUndeployed, d.State)
			assert.Empty(t, d.DeploymentID)
			assert.NotEqual(t, provider.Etag, d.Etag)
		}
	}
}

// intUntar reads a tar.gz into a map of file name to content.
func intUntar(t *testing.T, body []byte) map[string]string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(body))
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	files := map[string]string{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		files[h.Name] = string(content)
	}
	return files
}

// TestINTRoutesFetchBatchServesTheGatewaysDeploymentsAsTarGz pins
// fetch-batch: one directory per deployment of the calling gateway, other ids
// and unsynced kinds skipped, and its Accept and body checks.
func TestINTRoutesFetchBatchServesTheGatewaysDeploymentsAsTarGz(t *testing.T) {
	f := newINTSyncFixture(t)
	body := map[string]any{"deploymentIds": []string{
		f.providerDep.String(), f.agentDep.String(), f.restDep.String(), f.elsewhereDep.String(), uuid.NewString(), "not-a-uuid",
	}}

	rec := f.call(http.MethodPost, "/deployments/fetch-batch", body, withHeader("Accept", "application/x-tar+gzip"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/x-tar+gzip", rec.Header().Get("Content-Type"))
	files := intUntar(t, rec.Body.Bytes())
	assert.Len(t, files, 2)
	assert.Contains(t, files[f.providerDep.String()+"/llm-provider-"+f.provider.String()+".yaml"], "url: https://llm.example.com")
	assert.Contains(t, files[f.agentDep.String()+"/agent-"+f.agent.String()+".yaml"], "kind: Agent")

	rec = f.call(http.MethodPost, "/deployments/fetch-batch", map[string]any{"deploymentIds": []string{"not-a-uuid"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, intUntar(t, rec.Body.Bytes()), "no ids of the gateway: an empty archive")

	rec = f.call(http.MethodPost, "/deployments/fetch-batch", body, withHeader("Accept", "application/json"))
	require.Equal(t, http.StatusNotAcceptable, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "only serves application/x-tar+gzip")

	rec = f.call(http.MethodPost, "/deployments/fetch-batch", "{")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "Invalid request body")
}

// TestINTRoutesArtifactsExistAnswersPerID pins artifacts/exists: an artifact
// of the gateway's organization exists wherever it is deployed, an LLM proxy
// is also found by handle, anything else does not exist.
func TestINTRoutesArtifactsExistAnswersPerID(t *testing.T) {
	f := newINTSyncFixture(t)
	proxyHandle := unique("proxy")
	f.intArtifact(gwmodels.KindLLMProxy, proxyHandle)
	foreign := f.intArtifactIn(dbtest.Org(t, f.db), gwmodels.KindRestAPI, unique("foreign"))
	unknown := uuid.NewString()

	ids := []string{f.rest.String(), f.elsewhere.String(), proxyHandle, foreign.String(), unknown, "garbage"}
	got := expectInto[struct {
		Artifacts []struct {
			ArtifactID string `json:"artifactId"`
			Exists     bool   `json:"exists"`
		} `json:"artifacts"`
	}](t, f.call(http.MethodPost, "/artifacts/exists", map[string]any{"artifactIds": ids}), http.StatusOK)

	answers := map[string]bool{}
	order := make([]string, 0, len(got.Artifacts))
	for _, a := range got.Artifacts {
		answers[a.ArtifactID] = a.Exists
		order = append(order, a.ArtifactID)
	}
	assert.Equal(t, ids, order, "one answer per id, in the order asked")
	assert.Equal(t, map[string]bool{
		f.rest.String(): true, f.elsewhere.String(): true, proxyHandle: true,
		foreign.String(): false, unknown: false, "garbage": false,
	}, answers)

	rec := f.call(http.MethodPost, "/artifacts/exists", "{")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "Invalid request body")
}
