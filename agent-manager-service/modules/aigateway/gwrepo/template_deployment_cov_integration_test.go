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

package gwrepo

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// A provider template is created with its token extraction settings, read back by handle and id, listed and counted
// per organization, and deleted.
func TestCovLLMProviderTemplateRepoLifecycle(t *testing.T) {
	repo := NewLLMProviderTemplateRepo(db.GetDB())
	ou := newOrg(t)
	create := func(handle string) *gwmodels.LLMProviderTemplate {
		tpl := &gwmodels.LLMProviderTemplate{
			OUID: ou, Handle: handle, Name: handle,
			Metadata:     &gwmodels.LLMProviderTemplateMetadata{EndpointURL: "https://api.example.com"},
			PromptTokens: &gwmodels.ExtractionIdentifier{Location: "payload", Identifier: "$.usage.prompt_tokens"},
		}
		require.NoError(t, repo.Create(tpl))
		return tpl
	}
	first := create("first")
	create("second")
	require.Error(t, repo.Create(&gwmodels.LLMProviderTemplate{OUID: ou, Handle: "first", Name: "dup"}),
		"a handle is unique in an organization")

	got, err := repo.GetByHandle("first", ou)
	require.NoError(t, err)
	assert.Equal(t, first.UUID, got.UUID)
	require.NotNil(t, got.Metadata)
	assert.Equal(t, "https://api.example.com", got.Metadata.EndpointURL)
	require.NotNil(t, got.PromptTokens)
	assert.Equal(t, "$.usage.prompt_tokens", got.PromptTokens.Identifier)
	assert.Nil(t, got.CompletionTokens)
	byID, err := repo.GetByUUID(first.UUID.String(), ou)
	require.NoError(t, err)
	assert.Equal(t, "first", byID.Handle)
	_, err = repo.GetByHandle("first", newOrg(t))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = repo.GetByUUID(uuid.NewString(), ou)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	page, err := repo.List(ou, 1, 0)
	require.NoError(t, err)
	rest, err := repo.List(ou, 10, 1)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Len(t, rest, 1)
	assert.ElementsMatch(t, []string{"first", "second"}, []string{page[0].Handle, rest[0].Handle})
	n, err := repo.Count(ou)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	exists, err := repo.Exists("second", ou)
	require.NoError(t, err)
	assert.True(t, exists)

	require.ErrorIs(t, repo.Update(&gwmodels.LLMProviderTemplate{OUID: ou, Handle: "missing", Name: "x"}), gorm.ErrRecordNotFound)

	require.NoError(t, repo.Delete("second", ou))
	require.ErrorIs(t, repo.Delete("second", ou), gorm.ErrRecordNotFound)
	exists, err = repo.Exists("second", ou)
	require.NoError(t, err)
	assert.False(t, exists)
}

// covDeploy starts deploying the artifact to gateway and returns the deployment.
func covDeploy(t *testing.T, repo DeploymentRepository, ou string, artifact, gateway uuid.UUID, content string) *gwmodels.Deployment {
	t.Helper()
	d := &gwmodels.Deployment{Name: "d-" + uuid.NewString()[:8], Content: []byte(content)}
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return repo.StartDeploy(context.Background(), tx, gwmodels.Placement{
			ArtifactRef: gwmodels.ArtifactRef{OUID: ou, ArtifactUUID: artifact, Kind: gwmodels.KindAgent},
			GatewayUUID: gateway,
		}, d)
	}))
	return d
}

// A deployment reads back with its content or its state; a superseded one is archived; the gateways an artifact
// was sent to, and those a delete goes to, are listed in gateway order.
func TestCovDeploymentRepoReadsDeploymentsAndTheirGateways(t *testing.T) {
	gdb := db.GetDB()
	repo := NewDeploymentRepo(gdb)
	ou := newOrg(t)
	artifact := newArtifact(t, ou, gwmodels.KindAgent, "proj/agent/"+uuid.NewString())
	gw1, gw2 := newGateway(t, ou, gwmodels.GatewayRoleBoth), newGateway(t, ou, gwmodels.GatewayRoleBoth)
	idle := newGateway(t, ou, gwmodels.GatewayRoleBoth)
	require.NoError(t, NewGatewayRepo(gdb).UpdateActiveStatus(idle.String(), true))

	old := covDeploy(t, repo, ou, artifact, gw1, "kind: Agent\nrev: 1\n")
	current := covDeploy(t, repo, ou, artifact, gw1, "kind: Agent\nrev: 2\n")
	covDeploy(t, repo, ou, artifact, gw2, "kind: Agent\nrev: 1\n")

	withContent, err := repo.GetWithContent(current.DeploymentID.String(), artifact.String(), ou)
	require.NoError(t, err)
	assert.Equal(t, "kind: Agent\nrev: 2\n", string(withContent.Content))
	_, err = repo.GetWithContent(current.DeploymentID.String(), artifact.String(), newOrg(t))
	require.ErrorIs(t, err, gwerr.ErrDeploymentNotFound)

	state, err := repo.GetWithState(current.DeploymentID.String(), artifact.String(), ou)
	require.NoError(t, err)
	require.NotNil(t, state.Status)
	assert.Equal(t, gwmodels.DeploymentStatusDeploying, *state.Status)
	assert.Empty(t, state.Content, "the state read leaves the content out")
	archived, err := repo.GetWithStateCtx(context.Background(), old.DeploymentID.String(), artifact.String(), ou)
	require.NoError(t, err)
	require.NotNil(t, archived.Status)
	assert.Equal(t, gwmodels.DeploymentStatusArchived, *archived.Status, "a deployment no row points at is archived")
	unknown, err := repo.GetWithState(uuid.NewString(), artifact.String(), ou)
	require.ErrorIs(t, err, gwerr.ErrDeploymentNotFound, "an unknown deployment is not read back as an empty archived one")
	assert.Nil(t, unknown)
	_, err = repo.GetWithState(current.DeploymentID.String(), artifact.String(), newOrg(t))
	require.ErrorIs(t, err, gwerr.ErrDeploymentNotFound, "nor is another organization's")

	sent, err := repo.SentGateways(context.Background(), ou, artifact)
	require.NoError(t, err)
	want := []string{gw1.String(), gw2.String()}
	sort.Strings(want)
	assert.Equal(t, want, sent)
	deleteTo, err := repo.DeleteEventGateways(context.Background(), ou, artifact)
	require.NoError(t, err)
	want = append(want, idle.String())
	sort.Strings(want)
	assert.Equal(t, want, deleteTo, "a delete also goes to every connected gateway")

	rows, err := repo.StatusRecords(context.Background(), ou, artifact)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, sent[0], rows[0].GatewayUUID.String(), "ordered by gateway")
	none, err := repo.StatusRecords(context.Background(), ou, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, none)

	env := envOf(t, ou, "locked")
	require.NoError(t, NewGatewayRepo(gdb).CreateEnvironmentMapping(&gwmodels.GatewayEnvironmentMapping{GatewayUUID: gw1, EnvironmentUUID: env}))
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		mapped, err := repo.LockEnvironmentMapping(context.Background(), tx, gw1, env)
		require.NoError(t, err)
		assert.True(t, mapped)
		mapped, err = repo.LockEnvironmentMapping(context.Background(), tx, gw2, env)
		require.NoError(t, err)
		assert.False(t, mapped)
		return nil
	}))
}
