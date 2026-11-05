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

package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func content(context, vhost string) []byte {
	s := "apiVersion: gateway.api-platform.wso2.com/v1\nkind: RestApi\nspec:\n  context: " + context + "\n"
	if vhost != "" {
		s += "  vhost: " + vhost + "\n"
	}
	return []byte(s)
}

func TestRoutesOverlap(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"/a", "/a", true},
		{"/a", "/a/b", true},
		{"/a/b", "/a", true},
		{"/a", "/ab", false},
		{"/ab", "/a", false},
		{"/a/", "/a", true},
		{"/p1/weather", "/p1/weather-v2", false},
		{"/p1/weather", "/p1/weather/mcp", true},
		{"/", "/", true},
		{"", "/", true},
		{"/", "/anything", false},
		{"/anything", "/", false},
		{"/x", "/y", false},
	}
	for _, c := range cases {
		a, b := NewRoute(c.a, "", "gw.example.com"), NewRoute(c.b, "", "gw.example.com")
		assert.Equal(t, c.want, a.Overlaps(b), "%q overlaps %q", c.a, c.b)
	}
}

func TestRoutesOnDifferentVhostsNeverOverlap(t *testing.T) {
	gw := "https://gw.example.com/"
	assert.False(t, NewRoute("/a", "api.example.com", gw).Overlaps(NewRoute("/a", "", gw)))
	assert.True(t, NewRoute("/a", "gw.example.com", gw).Overlaps(NewRoute("/a", "", gw)),
		"an artifact vhost equal to the gateway's is the gateway's")
	assert.True(t, NewRoute("/a", "API.example.com", gw).Overlaps(NewRoute("/a/b", "http://api.example.com", gw)))
}

func TestRouteOf(t *testing.T) {
	assert.Equal(t, Route{Context: "/p1/weather", Vhost: "gw.example.com"}, RouteOf(content("/p1/weather/", ""), "gw.example.com"))
	assert.Equal(t, Route{Context: "/a", Vhost: "api.example.com"}, RouteOf(content("/a", "api.example.com"), "gw.example.com"))
	assert.Equal(t, Route{}, RouteOf([]byte("spec: {}\n"), "gw.example.com"), "no context, no route")
	assert.Equal(t, Route{}, RouteOf([]byte(":::"), "gw.example.com"))
}

func TestClash(t *testing.T) {
	gw := "gw.example.com"
	self, other := uuid.New(), uuid.New()
	deployed := func(rows ...models.GatewayDeploymentContent) []models.GatewayDeploymentContent { return rows }
	row := func(id uuid.UUID, c []byte) models.GatewayDeploymentContent {
		return models.GatewayDeploymentContent{ArtifactRef: models.ArtifactRef{ArtifactUUID: id}, Content: c}
	}

	t.Run("equal, nested and containing clash", func(t *testing.T) {
		for _, c := range []string{"/p1/weather", "/p1", "/p1/weather/tools"} {
			got, clash := Clash(self, NewRoute("/p1/weather", "", gw), deployed(row(other, content(c, ""))), gw)
			assert.True(t, clash, c)
			assert.Equal(t, c, got.Context)
		}
	})
	t.Run("a sibling is no clash", func(t *testing.T) {
		_, clash := Clash(self, NewRoute("/p1/weather", "", gw), deployed(row(other, content("/p1/weather-v2", ""))), gw)
		assert.False(t, clash)
	})
	t.Run("the artifact itself is excluded", func(t *testing.T) {
		_, clash := Clash(self, NewRoute("/p1/weather", "", gw), deployed(row(self, content("/p1", ""))), gw)
		assert.False(t, clash)
	})
	t.Run("a different vhost is no clash", func(t *testing.T) {
		_, clash := Clash(self, NewRoute("/a", "", gw), deployed(row(other, content("/a", "api.example.com"))), gw)
		assert.False(t, clash)
	})
	t.Run("root clashes only with root", func(t *testing.T) {
		_, clash := Clash(self, NewRoute("/a", "", gw), deployed(row(other, content("/", ""))), gw)
		assert.False(t, clash)
		_, clash = Clash(self, NewRoute("/", "", gw), deployed(row(other, content("/", ""))), gw)
		assert.True(t, clash)
	})
	t.Run("an unchanged route is grandfathered", func(t *testing.T) {
		rows := deployed(row(other, content("/a/b", "")), row(self, content("/a/", "")))
		_, clash := Clash(self, NewRoute("/a", "", gw), rows, gw)
		assert.False(t, clash, "the artifact already serves /a next to /a/b; a re-render at /a doesn't fail")
	})
	t.Run("a changed context is checked", func(t *testing.T) {
		rows := deployed(row(other, content("/b", "")), row(self, content("/a", "")))
		_, clash := Clash(self, NewRoute("/b/c", "", gw), rows, gw)
		assert.True(t, clash)
	})
	t.Run("a changed vhost is checked", func(t *testing.T) {
		rows := deployed(row(other, content("/a/b", "")), row(self, content("/a", "api.example.com")))
		_, clash := Clash(self, NewRoute("/a", "", gw), rows, gw)
		assert.True(t, clash)
	})
}

// clashRepo is a gateway holding one other artifact at otherContext, deployed.
func clashRepo(otherContext string) (*repomocks.DeploymentRepositoryMock, *int) {
	other := uuid.New()
	deployID := uuid.New()
	starts := 0
	repo := &repomocks.DeploymentRepositoryMock{
		ListForGatewaySyncFunc: func(context.Context, string, string) ([]models.GatewaySyncRow, error) {
			return []models.GatewaySyncRow{{
				ArtifactRef: models.ArtifactRef{ArtifactUUID: other}, DeploymentID: deployID.String(),
				State: models.GatewaySyncStateDeployed,
			}}, nil
		},
		ListContentForGatewayFunc: func(_ context.Context, _, _ string, ids []uuid.UUID) ([]models.GatewayDeploymentContent, error) {
			if len(ids) == 1 && ids[0] == deployID {
				return []models.GatewayDeploymentContent{{
					ArtifactRef: models.ArtifactRef{ArtifactUUID: other}, DeploymentID: deployID, Content: content(otherContext, ""),
				}}, nil
			}
			return nil, nil
		},
		StartDeployFunc: func(_ context.Context, _ *gorm.DB, _ models.Placement, d *models.Deployment) error {
			starts++
			now := time.Now()
			d.DeploymentID, d.UpdatedAt = uuid.New(), &now
			return nil
		},
	}
	repo.WithTxFunc = func(*gorm.DB) repositories.DeploymentRepository { return repo }
	return repo, &starts
}

// renderingKind renders every artifact at a fixed context.
type renderingKind struct {
	stubKind
	context string
}

func (k renderingKind) Render(context.Context, models.ArtifactRef) (Rendered, error) {
	return Rendered{Name: "r", Content: content(k.context, "")}, nil
}

type lockingGateways struct{ locks []string }

func (g *lockingGateways) AcquireGatewayLock(_ *gorm.DB, id string) error {
	g.locks = append(g.locks, id)
	return nil
}

func (g *lockingGateways) GetByUUID(id string) (*models.Gateway, error) {
	return &models.Gateway{UUID: uuid.MustParse(id), Vhost: "gw.example.com"}, nil
}

type noopHub struct{ eventhub.EventHub }

func (noopHub) PublishEventTx(context.Context, *gorm.DB, string, eventhub.Event) error { return nil }

func TestDeployRefusesAnOverlappingContext(t *testing.T) {
	repo, starts := clashRepo("/p1")
	gateways := &lockingGateways{}
	core := NewCore(nil, repo, noopHub{}, Kinds{renderingKind{context: "/p1/weather"}}, Config{}, discardLogger()).WithGateways(gateways)
	p := models.Placement{ArtifactRef: models.ArtifactRef{OUID: "ou", ArtifactUUID: uuid.New(), Kind: models.KindMCPMapping}, GatewayUUID: uuid.New()}

	_, err := core.Deploy(context.Background(), &gorm.DB{}, p)

	require.ErrorIs(t, err, utils.ErrContextInUse)
	require.ErrorIs(t, err, utils.ErrMCPContextInUse)
	assert.True(t, IsPermanent(err))
	assert.Zero(t, *starts, "nothing is deployed")
	assert.Equal(t, []string{p.GatewayUUID.String()}, gateways.locks, "the check runs under the gateway lock")
}

func TestDeployAllowsASiblingContext(t *testing.T) {
	repo, starts := clashRepo("/p1/weather-v2")
	core := NewCore(nil, repo, noopHub{}, Kinds{renderingKind{context: "/p1/weather"}}, Config{}, discardLogger()).WithGateways(&lockingGateways{})
	p := models.Placement{ArtifactRef: models.ArtifactRef{OUID: "ou", ArtifactUUID: uuid.New(), Kind: models.KindMCPMapping}, GatewayUUID: uuid.New()}

	_, err := core.Deploy(context.Background(), &gorm.DB{}, p)

	require.NoError(t, err)
	assert.Equal(t, 1, *starts)
}

func TestRenderReasonOfAClashIsContextInUse(t *testing.T) {
	assert.Equal(t, utils.ErrCodeContextInUse, renderReason(stubKind{}, Permanent(utils.ErrContextInUse)))
}
