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
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Gateways is what the core reads and locks gateways through: the gateway repository.
type Gateways interface {
	AcquireGatewayLock(tx *gorm.DB, gatewayID string) error
	GetByUUID(gatewayID string) (*models.Gateway, error)
}

// WithGateways makes the core refuse a deploy whose context clashes with another artifact's on its gateway. A core
// without gateways checks nothing.
func (c *Core) WithGateways(gateways Gateways) *Core {
	c.gateways = gateways
	return c
}

// Route is where a gateway serves an artifact: its normalized context on its effective vhost.
type Route struct {
	Context string
	Vhost   string
}

// NewRoute is the route of an artifact at context on vhost, or on the gateway's vhost when it names none.
func NewRoute(context, vhost, gatewayVhost string) Route {
	effective := normalizeVhost(vhost)
	if effective == "" {
		effective = normalizeVhost(gatewayVhost)
	}
	return Route{Context: normalizeContext(context), Vhost: effective}
}

// RouteOf is the route of a deployment's content: its spec.context and spec.vhost, the fields every kind AMS
// deploys (Mcp, LlmProvider, LlmProxy, RestApi, Agent) carries. The zero Route when it has no context.
func RouteOf(content []byte, gatewayVhost string) Route {
	var artifact struct {
		Spec struct {
			Context string `yaml:"context"`
			Vhost   string `yaml:"vhost"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(content, &artifact); err != nil || strings.TrimSpace(artifact.Spec.Context) == "" {
		return Route{}
	}
	return NewRoute(artifact.Spec.Context, artifact.Spec.Vhost, gatewayVhost)
}

// Overlaps reports whether two routes collide: on the same vhost, their contexts are equal or one is a path-segment
// prefix of the other (/a and /a/b do, /a and /ab don't). The root context collides only with itself: the gateway
// routes on the longest prefix, so it is a catch-all, not a shadow over every other context.
func (r Route) Overlaps(o Route) bool {
	if r.Vhost != o.Vhost {
		return false
	}
	if r.Context == o.Context {
		return true
	}
	if r.Context == "/" || o.Context == "/" {
		return false
	}
	return strings.HasPrefix(o.Context, r.Context+"/") || strings.HasPrefix(r.Context, o.Context+"/")
}

func normalizeContext(c string) string {
	c = strings.TrimRight(strings.TrimSpace(c), "/")
	if c == "" {
		return "/"
	}
	if !strings.HasPrefix(c, "/") {
		c = "/" + c
	}
	return c
}

// normalizeVhost is the host a vhost names: no scheme, no trailing slash, lower case.
func normalizeVhost(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.Index(v, "://"); i >= 0 {
		v = v[i+3:]
	}
	return strings.TrimRight(v, "/")
}

// Clash returns the route among deployed, the deployments a gateway holds, that next overlaps, skipping self's
// own. An artifact already deployed there at next is grandfathered: first deployed wins, so a re-render at an
// unchanged route never fails over an overlap that was already there.
func Clash(self uuid.UUID, next Route, deployed []models.GatewayDeploymentContent, gatewayVhost string) (Route, bool) {
	if next.Context == "" {
		return Route{}, false
	}
	for _, d := range deployed {
		if d.ArtifactUUID == self && RouteOf(d.Content, gatewayVhost) == next {
			return Route{}, false
		}
	}
	for _, d := range deployed {
		if d.ArtifactUUID == self {
			continue
		}
		if other := RouteOf(d.Content, gatewayVhost); other.Context != "" && other.Overlaps(next) {
			return other, true
		}
	}
	return Route{}, false
}

// CheckContext returns an error wrapping utils.ErrContextInUse when next, self's route on gateway, overlaps another
// artifact's there. It reads what the gateway is told to hold, its desired-deployed deployments, so an artifact
// placed but never sent is not seen. The caller holds the gateway lock for the check to be decisive.
func CheckContext(
	ctx context.Context, deployments repositories.DeploymentRepository, ouID string, gateway *models.Gateway,
	self uuid.UUID, next Route,
) error {
	if next.Context == "" {
		return nil
	}
	rows, err := deployments.ListForGatewaySync(ctx, ouID, gateway.UUID.String())
	if err != nil {
		return fmt.Errorf("failed to list the gateway's artifacts: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row.State != models.GatewaySyncStateDeployed {
			continue
		}
		if id, err := uuid.Parse(row.DeploymentID); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	contents, err := deployments.ListContentForGateway(ctx, ouID, gateway.UUID.String(), ids)
	if err != nil {
		return fmt.Errorf("failed to read the gateway's artifacts: %w", err)
	}
	if other, clash := Clash(self, next, contents, gateway.Vhost); clash {
		return fmt.Errorf("%w: %s overlaps %s on gateway %s", utils.ErrContextInUse, next.Context, other.Context, gateway.UUID)
	}
	return nil
}

// checkContext refuses, permanently, content whose route clashes on the placement's gateway. The caller has taken
// the gateway's lock in tx (lockGateway).
func (c *Core) checkContext(ctx context.Context, tx *gorm.DB, p models.Placement, content []byte) error {
	if c.gateways == nil {
		return nil
	}
	gateway, err := c.gateways.GetByUUID(p.GatewayUUID.String())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		gateway, err = &models.Gateway{UUID: p.GatewayUUID}, nil
	}
	if err != nil {
		return fmt.Errorf("failed to read gateway %s: %w", p.GatewayUUID, err)
	}
	if err := CheckContext(ctx, c.repo.WithTx(tx), p.OUID, gateway, p.ArtifactUUID, RouteOf(content, gateway.Vhost)); err != nil {
		if errors.Is(err, utils.ErrContextInUse) {
			return Permanent(err)
		}
		return err
	}
	return nil
}

// lockGateway takes the gateway's advisory lock in tx, serializing deploys on one gateway so two with overlapping
// contexts can't both pass the check. It is taken before the transaction's other gateway work, as the gateway
// mapping and binding paths take it.
func (c *Core) lockGateway(tx *gorm.DB, gateway uuid.UUID) error {
	if c.gateways == nil {
		return nil
	}
	if err := c.gateways.AcquireGatewayLock(tx, gateway.String()); err != nil {
		return fmt.Errorf("failed to lock gateway %s: %w", gateway, err)
	}
	return nil
}
