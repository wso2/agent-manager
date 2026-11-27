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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/api"
	"github.com/wso2/agent-manager/agent-manager-service/app"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aihub"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

const plansPath = "/m/ai-hub/subscription-plans"

type plansWorld struct {
	moduleWorld
	reader      http.Handler // may read plans only
	other       http.Handler // another organization, with every plan permission
	otherHandle string
	gateways    *planGateways // what the stand-in gateway module was sent, once installed
}

// planGateways records the plans and subscriptions the gateway handler would send to the organization's
// gateways, and the subscription changes the plans module published.
type planGateways struct {
	mu   sync.Mutex
	sent []sentPlan
	subs []subplanv1.SubscriptionChange
	// offers is how many MCP proxies offer each plan handle, as the stand-in gateway reports it.
	offers map[string]int
}

// offering is the stand-in gateway's count of the MCP proxies offering a plan (subplanv1.OfferCounter).
func (g *planGateways) offering(_ context.Context, _, handle string) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.offers[handle], nil
}

func (g *planGateways) setOffers(handle string, n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.offers == nil {
		g.offers = map[string]int{}
	}
	g.offers[handle] = n
}

// subscriptionChanged records a published subscription change, then hands it to the gateway's own handler.
func (g *planGateways) subscriptionChanged(plans func() (subplanv1.Source, bool)) modkit.EventHandler {
	send := gwsvc.SubscriptionChangeHandler(plans, standInDeployments{}, g, g)
	return func(ctx context.Context, e modkit.Event) error {
		var ch subplanv1.SubscriptionChange
		if err := json.Unmarshal(e.Data, &ch); err != nil {
			return err
		}
		g.mu.Lock()
		g.subs = append(g.subs, ch)
		g.mu.Unlock()
		return send(ctx, e)
	}
}

// SubscriptionChanged takes what the gateway handler sends; the published changes are recorded already.
func (g *planGateways) SubscriptionChanged(context.Context, string, subplanv1.Action, string, subplanv1.Subscription) error {
	return nil
}

// standInDeployments deploys the MCP proxy docs-mcp, which the portal deliveries in these tests are to, once.
type standInDeployments struct{}

const standInDeployment = "0b9e4c8e-5f0c-4a43-9a43-6f3cbb8e1d11"

func (standInDeployments) DeployedArtifactIDs(_ context.Context, _, _, handle string) ([]string, error) {
	if handle == "docs-mcp" {
		return []string{standInDeployment}, nil
	}
	return nil, nil
}

func (standInDeployments) DeployedProxies(context.Context, string) ([]string, error) {
	return []string{"docs-mcp"}, nil
}

func (standInDeployments) ProxyOf(_ context.Context, _, id string) (string, string, bool, error) {
	return subplanv1.ArtifactKindMCP, "docs-mcp", id == standInDeployment, nil
}

// subscriptionChange returns the last change published for the subscription with the action, if any.
func (g *planGateways) subscriptionChange(id string, action subplanv1.Action) (subplanv1.SubscriptionChange, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := len(g.subs) - 1; i >= 0; i-- {
		if g.subs[i].SubscriptionID == id && g.subs[i].Action == action {
			return g.subs[i], true
		}
	}
	return subplanv1.SubscriptionChange{}, false
}

type sentPlan struct {
	org    string
	action subplanv1.Action
	plan   subplanv1.Plan
}

func (g *planGateways) PlanChanged(_ context.Context, orgID string, action subplanv1.Action, p subplanv1.Plan) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sent = append(g.sent, sentPlan{org: orgID, action: action, plan: p})
	return nil
}

// find returns the last change sent for the plan handle with the action, if any.
func (g *planGateways) find(handle string, action subplanv1.Action) (sentPlan, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := len(g.sent) - 1; i >= 0; i-- {
		if g.sent[i].plan.Handle == handle && g.sent[i].action == action {
			return g.sent[i], true
		}
	}
	return sentPlan{}, false
}

// planGatewayModule stands in for the AI Gateway: it subscribes to the plans module's subscription event with
// the gateway's own handler (gwsvc.SubscriptionChangeHandler) and records what it would send to the
// organization's gateways. It provides planGatewayReady so a test can wait for it.
type planGatewayModule struct{ gateways *planGateways }

const (
	planGatewayModuleName = "plangw"
	planGatewayReady      = "plangw.ready/v1"
)

func (m planGatewayModule) Manifest() modkit.Manifest {
	return modkit.Manifest{
		Name: planGatewayModuleName, Version: "0.1.0", CoreAPI: modkit.CoreAPIVersion,
		Provides: []string{planGatewayReady},
		Optional: []string{subplanv1.ID},
	}
}
func (planGatewayModule) Migrations() fs.FS { return nil }
func (planGatewayModule) UI() fs.FS         { return nil }
func (m planGatewayModule) Activate(_ context.Context, rt modkit.Runtime) error {
	rt.Provide(planGatewayReady, true)
	plans := func() (subplanv1.Source, bool) { return modkit.Lookup[subplanv1.Source](rt, subplanv1.ID) }
	// As the AI Gateway does: subscribe to the event the plans module names, while it is active.
	if source, ok := plans(); ok {
		rt.Subscribe(source.SubscriptionChangeEvent(), m.gateways.subscriptionChanged(plans))
		source.CountOffersWith(m.gateways.offering)
	}
	return nil
}

func newPlansWorld(t *testing.T) plansWorld {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	database := db.DB(context.Background())
	gateways := &planGateways{}
	host := modhost.NewWith(database, config.GetConfig().Database.Driver, slog.New(slog.DiscardHandler),
		[]modkit.Module{aihub.Module(), planGatewayModule{gateways: gateways}})
	host.PollInterval = 50 * time.Millisecond
	_, err := host.Migrate(ctx)
	require.NoError(t, err)

	params := func(auth jwtassertion.Middleware) *wiring.AppParams {
		p, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), database, auth,
			wiring.TestClients{PlatformRepository: repositories.NewPlatformRepo(database)})
		require.NoError(t, err)
		return p
	}
	newOrg := func() (string, string) {
		id := uuid.Must(uuid.NewV7()).String()
		return id, "plans-" + id[len(id)-8:]
	}
	org, handle := newOrg()
	otherOrg, otherHandle := newOrg()
	full := params(moduleAuth(org, handle, "amp:ai-hub:plan-read", "amp:ai-hub:plan-create", "amp:ai-hub:plan-update", "amp:ai-hub:plan-delete"))
	reader := params(moduleAuth(org, handle, "amp:ai-hub:plan-read"))
	limited := params(moduleAuth(org, handle))
	other := params(moduleAuth(otherOrg, otherHandle, "amp:ai-hub:plan-read", "amp:ai-hub:plan-create", "amp:ai-hub:plan-update", "amp:ai-hub:plan-delete"))

	stop, err := app.StartModules(ctx, host, database, full)
	require.NoError(t, err)
	t.Cleanup(func() {
		stop()
		cancel()
		coreext.Set(nil)
		rbac.SetCurrent(rbac.NewRegistry())
	})
	handler := func(p *wiring.AppParams) http.Handler { return api.MakeHTTPHandlerWithModules(p, nil, nil, host) }
	// The module is not installed: tests install it (installPlans).
	return plansWorld{
		moduleWorld: moduleWorld{
			host: host, full: handler(full), limited: handler(limited), org: org, handle: handle,
			internal: api.MakeInternalHTTPHandlerWithModules(full, host),
		},
		reader:      handler(reader),
		other:       handler(other),
		otherHandle: otherHandle,
		gateways:    gateways,
	}
}

// planDoc is a plan document to send: metadata.name is the handle, spec the rest.
func planDoc(name string, spec map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1", "kind": "SubscriptionPlan",
		"metadata": map[string]any{"name": name}, "spec": spec,
	}
}

type planUsage struct {
	InUse         bool `json:"inUse"`
	MCPProxies    int  `json:"mcpProxies"`
	Subscriptions int  `json:"subscriptions"`
}

// planBody is a plan document as the API returns it, read flat: ID is metadata.name.
type planBody struct {
	APIVersion  string
	Kind        string
	ID          string
	UID         string
	DisplayName string
	Description string
	Conditions  []json.RawMessage
	ExpiryTime  *time.Time
	Status      string
	CreatedBy   string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Usage       *planUsage
	Origin      string
	// Deprecated and DeprecatedAt are status.deprecated and status.deprecationTimestamp.
	Deprecated   bool
	DeprecatedAt *time.Time
}

func (p *planBody) UnmarshalJSON(b []byte) error {
	var doc struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Name              string     `json:"name"`
			UID               string     `json:"uid"`
			CreatedBy         string     `json:"createdBy"`
			CreationTimestamp *time.Time `json:"creationTimestamp"`
			UpdateTimestamp   *time.Time `json:"updateTimestamp"`
			Origin            string     `json:"origin"`
		} `json:"metadata"`
		Spec struct {
			DisplayName string            `json:"displayName"`
			Description string            `json:"description"`
			Status      string            `json:"status"`
			Conditions  []json.RawMessage `json:"conditions"`
			ExpiryTime  *time.Time        `json:"expiryTime"`
		} `json:"spec"`
		Status *struct {
			Usage                *planUsage `json:"usage"`
			Deprecated           bool       `json:"deprecated"`
			DeprecationTimestamp *time.Time `json:"deprecationTimestamp"`
		} `json:"status"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	*p = planBody{
		APIVersion: doc.APIVersion, Kind: doc.Kind, ID: doc.Metadata.Name, UID: doc.Metadata.UID,
		DisplayName: doc.Spec.DisplayName, Description: doc.Spec.Description, Conditions: doc.Spec.Conditions,
		ExpiryTime: doc.Spec.ExpiryTime, Status: doc.Spec.Status, CreatedBy: doc.Metadata.CreatedBy,
		CreatedAt: doc.Metadata.CreationTimestamp, UpdatedAt: doc.Metadata.UpdateTimestamp,
		Origin: doc.Metadata.Origin,
	}
	if doc.Status != nil {
		p.Usage, p.Deprecated, p.DeprecatedAt = doc.Status.Usage, doc.Status.Deprecated, doc.Status.DeprecationTimestamp
	}
	return nil
}

type errBody struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TestSubscriptionPlansModule drives the subscription plans module's API
// (/subscription-plans) through the core's module host, sign-in, permission checks and audit. The
// module is built in, so it is active without an install.
func TestSubscriptionPlansModule(t *testing.T) {
	w := newPlansWorld(t)
	ctx := context.Background()
	goldConditions := []any{
		map[string]any{"type": "request-count", "params": map[string]any{"limits": []any{
			map[string]any{"count": 1000, "timeUnit": "HOUR"},
		}}},
		map[string]any{"type": "tool-call-count", "params": map[string]any{"limits": []any{
			map[string]any{"count": 3, "timeAmount": 1, "timeUnit": "MINUTE"},
		}, "onExceed": "reject"}},
	}
	gold := planDoc("gold", map[string]any{
		"displayName": "Gold", "description": "For production agents", "conditions": goldConditions,
	})

	// Creating a project creates the organization in the core: plans need their organization to exist.
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/projects",
		projectDocument("plans", "Plans", nil))
	require.Equal(t, http.StatusAccepted, code, string(body))
	code, body = w.call(t, w.other, http.MethodPost, "/api/v1/projects",
		projectDocument("plans", "Plans", nil))
	require.Equal(t, http.StatusAccepted, code, string(body))

	t.Run("installable: not served until it is installed", func(t *testing.T) {
		deactivatePlans(ctx, t, w)
		installPlans(ctx, t, w)
	})

	t.Run("needs the plan permissions", func(t *testing.T) {
		code, _ := w.call(t, w.limited, http.MethodGet, plansPath, nil)
		assert.Equal(t, http.StatusForbidden, code)
		code, _ = w.call(t, w.reader, http.MethodPost, plansPath, gold)
		assert.Equal(t, http.StatusForbidden, code)
	})

	t.Run("create fills defaults and returns the whole document", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, gold)
		require.Equal(t, http.StatusCreated, code, string(body))
		p := decodeAs[planBody](t, body)
		assert.Equal(t, "ai-control-plane.choreolabs.com/v1", p.APIVersion)
		assert.Equal(t, "SubscriptionPlan", p.Kind)
		assert.Equal(t, "gold", p.ID)
		assert.NotEmpty(t, p.UID)
		assert.Equal(t, "ACTIVE", p.Status)
		assert.Equal(t, "module-tester", p.CreatedBy)
		require.NotNil(t, p.CreatedAt)
		require.NotNil(t, p.UpdatedAt)
		require.Len(t, p.Conditions, 2)
		assert.JSONEq(t, `{"id":"requests","type":"request-count","params":{"limits":[{"count":1000,"timeAmount":1,"timeUnit":"HOUR"}],"onExceed":"reject"}}`,
			string(p.Conditions[0]))
		assert.NotContains(t, string(body), `"limits":[{"limitCount"`, "the older limits field is taken, not returned")
	})

	t.Run("a plan made here has the control plane as its origin", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, plansPath+"/gold", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		p := decodeAs[planBody](t, body)
		assert.Equal(t, aihub.OriginControlPlane, p.Origin)
		assert.False(t, p.Deprecated)
	})

	t.Run("a document read back can be sent back as it is", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, plansPath+"/gold", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var doc map[string]any
		require.NoError(t, json.Unmarshal(body, &doc))
		code, body = w.call(t, w.full, http.MethodPut, plansPath+"/gold", doc)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.Len(t, decodeAs[planBody](t, body).Conditions, 2)
	})

	t.Run("YAML in and out", func(t *testing.T) {
		yamlDoc := "apiVersion: ai-control-plane.choreolabs.com/v1\nkind: SubscriptionPlan\nmetadata:\n  name: yaml-plan\n" +
			"spec:\n  displayName: From YAML\n  expiryTime: 2030-01-01T00:00:00Z\n  conditions:\n" +
			"    - type: tool-call-count\n      params:\n        limits: [{count: 5, timeUnit: MINUTE}]\n"
		req := httptest.NewRequest(http.MethodPost, plansPath, strings.NewReader(yamlDoc))
		req.Header.Set("Content-Type", "application/yaml")
		req.Header.Set("Accept", "application/yaml")
		rec := httptest.NewRecorder()
		w.full.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.Equal(t, "application/yaml", rec.Header().Get("Content-Type"))
		out := rec.Body.String()
		assert.Contains(t, out, "kind: SubscriptionPlan")
		assert.Contains(t, out, "name: yaml-plan")
		assert.Contains(t, out, "displayName: From YAML")
		assert.Less(t, strings.Index(out, "apiVersion:"), strings.Index(out, "spec:"))

		code, body := w.call(t, w.full, http.MethodGet, plansPath+"/yaml-plan", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		p := decodeAs[planBody](t, body)
		require.NotNil(t, p.ExpiryTime)
		assert.Equal(t, 2030, p.ExpiryTime.Year())
		code, _ = w.call(t, w.full, http.MethodDelete, plansPath+"/yaml-plan", nil)
		require.Equal(t, http.StatusOK, code, "deprecated")
	})

	t.Run("create refuses a duplicate, bad input and pricing", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, gold)
		assert.Equal(t, http.StatusConflict, code)
		assert.Equal(t, "SUBSCRIPTION_PLAN_EXISTS", decodeAs[errBody](t, body).Code)

		x := func(spec map[string]any) map[string]any { return planDoc("x", spec) }
		with := func(doc map[string]any, key string, v any) map[string]any {
			out := map[string]any{}
			for k, val := range doc {
				out[k] = val
			}
			if v == nil {
				delete(out, key)
			} else {
				out[key] = v
			}
			return out
		}
		for name, c := range map[string]struct {
			doc  map[string]any
			says string
		}{
			"no metadata.name":       {planDoc("", map[string]any{"displayName": "X"}), "metadata.name"},
			"long name":              {planDoc("a234567890123456789012345678901234567890x", map[string]any{"displayName": "X"}), "metadata.name"},
			"bad name":               {planDoc("Gold Plan", map[string]any{"displayName": "X"}), "metadata.name"},
			"wrong kind":             {with(x(map[string]any{"displayName": "X"}), "kind", "LlmProvider"), "kind"},
			"wrong apiVersion":       {with(x(map[string]any{"displayName": "X"}), "apiVersion", "v1"), "apiVersion"},
			"no spec":                {with(x(nil), "spec", nil), "spec"},
			"no displayName":         {x(map[string]any{"description": "X"}), "spec.displayName"},
			"bad status":             {x(map[string]any{"displayName": "X", "status": "DRAFT"}), "status"},
			"conditions and limits":  {x(map[string]any{"displayName": "X", "conditions": []any{}, "limits": []any{}}), "conditions or limits"},
			"two request conditions": {x(map[string]any{"displayName": "X", "conditions": []any{goldConditions[0], goldConditions[0]}}), ""},
			"bad expiry":             {x(map[string]any{"displayName": "X", "expiryTime": "tomorrow"}), "expiryTime"},
			"pricing":                {x(map[string]any{"displayName": "X", "pricing": map[string]any{"pricingType": "FLAT_RATE"}}), "pricing"},
		} {
			code, body := w.call(t, w.full, http.MethodPost, plansPath, c.doc)
			assert.Equal(t, http.StatusBadRequest, code, name)
			e := decodeAs[errBody](t, body)
			assert.Equal(t, "VALIDATION_FAILED", e.Code, name)
			assert.Equal(t, "error", e.Status, name)
			assert.Contains(t, e.Message, c.says, name)
		}
	})

	t.Run("old limits become a request-count condition", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, planDoc("legacy", map[string]any{
			"displayName": "Legacy", "status": "INACTIVE",
			"limits": []any{map[string]any{"timeUnit": "MINUTE", "limitCount": 10, "stopOnQuotaReach": false}},
		}))
		require.Equal(t, http.StatusCreated, code, string(body))
		p := decodeAs[planBody](t, body)
		require.Len(t, p.Conditions, 1)
		assert.JSONEq(t, `{"id":"requests","type":"request-count","params":{"limits":[{"count":10,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"allow"}}`,
			string(p.Conditions[0]))
	})

	t.Run("get carries usage in status", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, plansPath+"/gold", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		p := decodeAs[planBody](t, body)
		require.NotNil(t, p.Usage)
		assert.False(t, p.Usage.InUse)
		code, body = w.call(t, w.full, http.MethodGet, plansPath+"/missing", nil)
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "SUBSCRIPTION_PLAN_NOT_FOUND", decodeAs[errBody](t, body).Code)
	})

	t.Run("list pages, searches and filters, newest first", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, plansPath, nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var list struct {
			List       []planBody `json:"list"`
			Count      int        `json:"count"`
			Pagination struct{ Total, Offset, Limit int }
		}
		require.NoError(t, json.Unmarshal(body, &list))
		assert.Equal(t, 3, list.Count, "gold, the deprecated yaml-plan and legacy")
		assert.Equal(t, 3, list.Pagination.Total)
		assert.Equal(t, 20, list.Pagination.Limit)
		require.Len(t, list.List, 3)
		assert.Equal(t, []string{"legacy", "gold", "yaml-plan"}, []string{list.List[0].ID, list.List[1].ID, list.List[2].ID},
			"current plans newest first, then the deprecated one")
		assert.Equal(t, "SubscriptionPlan", list.List[0].Kind, "each item is a whole document")
		assert.Nil(t, list.List[0].Usage, "list items leave out usage")
		assert.True(t, list.List[2].Deprecated, "a deprecated plan is listed, marked deprecated")

		_, body = w.call(t, w.full, http.MethodGet, plansPath+"?limit=2&offset=2", nil)
		require.NoError(t, json.Unmarshal(body, &list))
		require.Len(t, list.List, 1, "the order holds across pages")
		assert.Equal(t, "yaml-plan", list.List[0].ID)

		_, body = w.call(t, w.full, http.MethodGet, plansPath+"?query=GOL", nil)
		require.NoError(t, json.Unmarshal(body, &list))
		require.Equal(t, 1, list.Count)
		assert.Equal(t, "gold", list.List[0].ID)

		code, body = w.call(t, w.full, http.MethodGet, plansPath+"?query=%25", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var literal struct{ Count int }
		require.NoError(t, json.Unmarshal(body, &literal))
		assert.Equal(t, 0, literal.Count, "% is taken literally")

		_, body = w.call(t, w.full, http.MethodGet, plansPath+"?status=inactive&limit=1&offset=0", nil)
		require.NoError(t, json.Unmarshal(body, &list))
		require.Equal(t, 1, list.Count)
		assert.Equal(t, "legacy", list.List[0].ID)
		assert.Equal(t, 1, list.Pagination.Limit)

		code, _ = w.call(t, w.full, http.MethodGet, plansPath+"?status=DRAFT", nil)
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("update keeps omitted fields and clears expiry on null", func(t *testing.T) {
		expiry := "2030-01-01T00:00:00Z"
		code, body := w.call(t, w.full, http.MethodPut, plansPath+"/gold", planDoc("gold", map[string]any{
			"displayName": "Gold+", "expiryTime": expiry, "status": "INACTIVE",
		}))
		require.Equal(t, http.StatusOK, code, string(body))
		p := decodeAs[planBody](t, body)
		assert.Equal(t, "Gold+", p.DisplayName)
		assert.Equal(t, "For production agents", p.Description, "an omitted description is kept")
		assert.Len(t, p.Conditions, 2, "omitted conditions are kept")
		require.NotNil(t, p.ExpiryTime)
		assert.Equal(t, "INACTIVE", p.Status)

		code, body = w.call(t, w.full, http.MethodPut, plansPath+"/gold", planDoc("gold", map[string]any{
			"displayName": "Gold+", "expiryTime": nil, "conditions": []any{},
		}))
		require.Equal(t, http.StatusOK, code, string(body))
		p = decodeAs[planBody](t, body)
		assert.Nil(t, p.ExpiryTime)
		assert.Empty(t, p.Conditions, "[] is a plan with no conditions")
		assert.Equal(t, "INACTIVE", p.Status, "an omitted status is kept")

		for name, b := range map[string]map[string]any{
			"no name":       planDoc("gold", map[string]any{"description": "x"}),
			"name change":   planDoc("silver", map[string]any{"displayName": "Gold"}),
			"pricing":       planDoc("gold", map[string]any{"displayName": "Gold", "pricing": nil}),
			"null conds":    planDoc("gold", map[string]any{"displayName": "Gold", "conditions": nil}),
			"bad condition": planDoc("gold", map[string]any{"displayName": "Gold", "conditions": []any{map[string]any{"type": "event-count"}}}),
		} {
			code, _ := w.call(t, w.full, http.MethodPut, plansPath+"/gold", b)
			assert.Equal(t, http.StatusBadRequest, code, name)
		}
		code, _ = w.call(t, w.full, http.MethodPut, plansPath+"/missing", planDoc("missing", map[string]any{"displayName": "X"}))
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("plans belong to one organization", func(t *testing.T) {
		code, _ := w.call(t, w.other, http.MethodGet, plansPath+"/gold", nil)
		assert.Equal(t, http.StatusNotFound, code)
		code, body := w.call(t, w.other, http.MethodPost, plansPath, gold)
		assert.Equal(t, http.StatusCreated, code, "the same handle in another org: "+string(body))
		code, _ = w.call(t, w.other, http.MethodDelete, plansPath+"/legacy", nil)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("search folds case beyond ASCII, even where folding lengthens the name", func(t *testing.T) {
		long := strings.Repeat("ß", 200) // folds to 400 characters
		code, body := w.call(t, w.full, http.MethodPost, plansPath, planDoc("strasse", map[string]any{"displayName": long}))
		require.Equal(t, http.StatusCreated, code, string(body))
		code, body = w.call(t, w.full, http.MethodPost, plansPath, planDoc("equipe", map[string]any{"displayName": "Équipe"}))
		require.Equal(t, http.StatusCreated, code, string(body))
		for query, want := range map[string]string{"SSSS": "strasse", "%C3%A9QUIPE": "equipe"} {
			code, body = w.call(t, w.full, http.MethodGet, plansPath+"?query="+query, nil)
			require.Equal(t, http.StatusOK, code, string(body))
			var found struct {
				List []planBody `json:"list"`
			}
			require.NoError(t, json.Unmarshal(body, &found))
			require.Len(t, found.List, 1, query)
			assert.Equal(t, want, found.List[0].ID, query)
		}
		for _, id := range []string{"strasse", "equipe"} {
			code, _ = w.call(t, w.full, http.MethodDelete, plansPath+"/"+id, nil)
			require.Equal(t, http.StatusOK, code)
		}
	})

	t.Run("deprecate keeps the plan, read only, and closes its handle", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodDelete, plansPath+"/legacy", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		p := decodeAs[planBody](t, body)
		assert.True(t, p.Deprecated)
		require.NotNil(t, p.DeprecatedAt)

		code, body = w.call(t, w.full, http.MethodGet, plansPath+"/legacy", nil)
		require.Equal(t, http.StatusOK, code, "a deprecated plan is still there: "+string(body))
		assert.True(t, decodeAs[planBody](t, body).Deprecated)
		assert.Len(t, decodeAs[planBody](t, body).Conditions, 1, "with its conditions")

		for name, spec := range map[string]map[string]any{
			"description": {"displayName": "Legacy", "description": "x"},
			"status":      {"displayName": "Legacy", "status": "ACTIVE"},
		} {
			code, body = w.call(t, w.full, http.MethodPut, plansPath+"/legacy", planDoc("legacy", spec))
			assert.Equal(t, http.StatusConflict, code, name)
			assert.Equal(t, "SUBSCRIPTION_PLAN_DEPRECATED", decodeAs[errBody](t, body).Code, name)
		}
		code, body = w.call(t, w.full, http.MethodDelete, plansPath+"/legacy", nil)
		assert.Equal(t, http.StatusOK, code, "deprecating it again changes nothing: "+string(body))
		assert.Equal(t, p.DeprecatedAt.Unix(), decodeAs[planBody](t, body).DeprecatedAt.Unix())
		code, body = w.call(t, w.full, http.MethodPost, plansPath, planDoc("legacy", map[string]any{"displayName": "Legacy"}))
		assert.Equal(t, http.StatusConflict, code, "its handle stays taken: "+string(body))
		code, _ = w.call(t, w.full, http.MethodDelete, plansPath+"/missing", nil)
		assert.Equal(t, http.StatusNotFound, code)
		var plans int64
		require.NoError(t, db.DB(ctx).Raw(`SELECT COUNT(*) FROM mod_ai_hub_subscription_plans WHERE org_id = ? AND handle = ?`,
			w.org, "legacy").Scan(&plans).Error)
		assert.EqualValues(t, 1, plans, "nothing is removed")
	})
}

const webhookSecret = "integration-webhook-secret"

// delivery is a signed webhook delivery for a plan event, as a sender builds it.
func delivery(t *testing.T, org, eventType string, data map[string]any, occurredAt time.Time, signedAt time.Time) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"event_id": uuid.NewString(), "event_type": eventType, "occurred_at": occurredAt.UTC().Format(time.RFC3339Nano),
		"org": map[string]any{"ref_id": org}, "aggregate_type": "subscription_plan",
		"secret_hash": aihub.SecretHash(webhookSecret), "encrypted_fields": []string{}, "data": data,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, aihub.WebhookRoutePath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(aihub.DefaultWebhookSignatureHeader, aihub.Sign(webhookSecret, body, signedAt))
	return req
}

func send(t *testing.T, h http.Handler, req *http.Request) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// TestSubscriptionPlanWebhook sends signed plan deliveries to the internal server and checks the plans
// they leave, through the plans API.
func TestSubscriptionPlanWebhook(t *testing.T) {
	t.Setenv("MODULE_AI_HUB_WEBHOOK_SECRET", webhookSecret)
	w := newPlansWorld(t)
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/projects",
		projectDocument("hooks", "Hooks", nil))
	require.Equal(t, http.StatusAccepted, code, body)
	now := time.Now()
	gone := map[string]any{"handle": "never-copied", "display_name": "Never copied"}
	deactivatePlans(context.Background(), t, w)
	code, refused := send(t, w.internal, delivery(t, w.org, "subscription_plan.created", gone, now, now))
	// A module's webhooks, like its other internal routes, answer module_inactive while it is not active.
	require.Equal(t, http.StatusUnauthorized, code, "a module that isn't active answers no delivery: "+refused)
	assert.Contains(t, refused, "module_inactive")
	installPlans(context.Background(), t, w)
	mcpPlan := map[string]any{
		"handle": "mcp-gold", "display_name": "MCP Gold", "description": "From the MCP hub",
		"conditions": []any{map[string]any{"type": "tool-call-count", "params": map[string]any{
			"limits": []any{map[string]any{"count": 30, "timeAmount": 1, "timeUnit": "MINUTE"}},
		}}},
		"pricing": nil,
	}

	t.Run("created: the plan is copied in, dated when it was made", func(t *testing.T) {
		made := now.Add(-time.Hour)
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.created", mcpPlan, made, now))
		require.Equal(t, http.StatusOK, code, body)
		assert.JSONEq(t, `{"status":"processed"}`, body)
		code, got := w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		require.Equal(t, http.StatusOK, code, string(got))
		p := decodeAs[planBody](t, got)
		assert.Equal(t, "MCP Gold", p.DisplayName)
		assert.Equal(t, "ACTIVE", p.Status)
		assert.Equal(t, aihub.DefaultWebhookSourceName, p.Origin, "the developer portal made it")
		require.Len(t, p.Conditions, 1)
		assert.JSONEq(t, `{"id":"tool-calls","type":"tool-call-count","params":{"limits":[{"count":30,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"reject"}}`,
			string(p.Conditions[0]))
	})

	t.Run("a repeated delivery changes nothing", func(t *testing.T) {
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.created", mcpPlan, now, now))
		require.Equal(t, http.StatusOK, code, body)
		_, got := w.call(t, w.full, http.MethodGet, plansPath+"?query=mcp-gold", nil)
		assert.Contains(t, string(got), `"count":1`)
	})

	t.Run("updated: name and conditions follow; old senders' limits change only requests", func(t *testing.T) {
		upd := map[string]any{
			"handle": "mcp-gold", "display_name": "MCP Gold+",
			"limits": []any{map[string]any{"limit_type": "REQUEST_COUNT", "limit_count": 100, "time_unit": "MINUTE", "time_amount": 1}},
		}
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.updated", upd, now, now))
		require.Equal(t, http.StatusOK, code, body)
		_, got := w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		p := decodeAs[planBody](t, got)
		assert.Equal(t, "MCP Gold+", p.DisplayName)
		require.Len(t, p.Conditions, 2, "the request limit is added; the tool-call condition stays")
	})

	t.Run("an update made at the source before the last one changes nothing", func(t *testing.T) {
		older := map[string]any{"handle": "mcp-gold", "display_name": "MCP Gold (older)"}
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.updated", older, now.Add(-30*time.Minute), now))
		require.Equal(t, http.StatusOK, code, "an out-of-date event is processed, and ignored: "+body)
		_, got := w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		assert.Equal(t, "MCP Gold+", decodeAs[planBody](t, got).DisplayName)
	})

	t.Run("refused alike: bad signature, stale signature, unknown org, org by handle, unknown event", func(t *testing.T) {
		bad := delivery(t, w.org, "subscription_plan.created", mcpPlan, now, now)
		bad.Header.Set(aihub.DefaultWebhookSignatureHeader, aihub.Sign("wrong-secret", []byte("x"), now))
		stale := delivery(t, w.org, "subscription_plan.created", mcpPlan, now, now.Add(-10*time.Minute))
		unknownOrg := delivery(t, uuid.NewString(), "subscription_plan.created", mcpPlan, now, now)
		byHandle := delivery(t, w.handle, "subscription_plan.created", mcpPlan, now, now)
		unknownEvent := delivery(t, w.org, "meter.created", map[string]any{"handle": "m"}, now, now)
		for name, req := range map[string]*http.Request{
			"bad signature": bad, "stale signature": stale, "unknown org": unknownOrg, "org by its handle": byHandle,
			"unknown event": unknownEvent,
		} {
			code, body := send(t, w.internal, req)
			assert.Equal(t, http.StatusUnauthorized, code, name)
			assert.JSONEq(t, `{"status":"error","code":"UNAUTHORIZED","message":"Invalid or expired credentials."}`, body, name)
		}
	})

	t.Run("a plan made here can't be created again by the portal", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, planDoc("cp-made", map[string]any{"displayName": "CP made"}))
		require.Equal(t, http.StatusCreated, code, string(body))
		code, got := send(t, w.internal, delivery(t, w.org, "subscription_plan.created",
			map[string]any{"handle": "cp-made", "display_name": "Portal copy"}, now, now))
		assert.Equal(t, http.StatusConflict, code, "a 409, which the portal doesn't retry: "+got)
		assert.Contains(t, got, "SUBSCRIPTION_PLAN_ORIGIN_CONFLICT")
		_, body = w.call(t, w.full, http.MethodGet, plansPath+"/cp-made", nil)
		assert.Equal(t, "CP made", decodeAs[planBody](t, body).DisplayName)
	})

	t.Run("a priced plan is stored without its price", func(t *testing.T) {
		paid := map[string]any{
			"handle": "paid", "display_name": "Paid", "pricing": map[string]any{"pricingType": "FLAT_RATE"},
			"conditions": []any{map[string]any{"type": "request-count", "params": map[string]any{
				"limits": []any{map[string]any{"count": 10, "timeAmount": 1, "timeUnit": "MINUTE"}},
			}}},
		}
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.created", paid, now, now))
		require.Equal(t, http.StatusOK, code, body)
		code, got := w.call(t, w.full, http.MethodGet, plansPath+"/paid", nil)
		require.Equal(t, http.StatusOK, code, string(got))
		p := decodeAs[planBody](t, got)
		assert.Equal(t, "Paid", p.DisplayName)
		assert.Len(t, p.Conditions, 1, "its conditions are kept")
		assert.NotContains(t, string(got), "pricing")
	})

	t.Run("deleted: an older deletion keeps a newer plan; a current one deprecates it; a repeat succeeds", func(t *testing.T) {
		del := map[string]any{"handle": "mcp-gold"}
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.deleted", del, now.Add(-2*time.Hour), now))
		require.Equal(t, http.StatusOK, code, body)
		_, got := w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		assert.False(t, decodeAs[planBody](t, got).Deprecated, "the plan was made after that deletion")

		code, body = send(t, w.internal, delivery(t, w.org, "subscription_plan.deleted", del, now, now))
		require.Equal(t, http.StatusOK, code, body)
		code, got = w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		require.Equal(t, http.StatusOK, code, "a deleted plan is deprecated, not removed: "+string(got))
		p := decodeAs[planBody](t, got)
		assert.True(t, p.Deprecated)
		require.NotNil(t, p.DeprecatedAt)
		assert.WithinDuration(t, now, *p.DeprecatedAt, time.Second, "dated by the portal's clock")

		code, body = send(t, w.internal, delivery(t, w.org, "subscription_plan.deleted", del, now, now))
		assert.Equal(t, http.StatusOK, code, "a plan already deprecated stays so: "+body)
	})

	t.Run("a deprecated plan doesn't change, and a plan made again with its handle is refused", func(t *testing.T) {
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.updated", mcpPlan, now.Add(time.Minute), now))
		require.Equal(t, http.StatusOK, code, body)
		_, got := w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		p := decodeAs[planBody](t, got)
		assert.True(t, p.Deprecated)
		assert.Equal(t, "MCP Gold+", p.DisplayName, "an update of a deprecated plan is ignored")

		code, body = send(t, w.internal, delivery(t, w.org, "subscription_plan.created", mcpPlan, now.Add(time.Minute), now))
		assert.Equal(t, http.StatusConflict, code, body)
		assert.Contains(t, body, "SUBSCRIPTION_PLAN_DEPRECATED")
	})

	t.Run("disabled: neither the API nor the webhook answers", func(t *testing.T) {
		require.NoError(t, w.host.Disable(context.Background(), aihub.Name))
		eventually(t, "the module routes to go", func() bool {
			code, _ := w.call(t, w.full, http.MethodGet, plansPath, nil)
			return code == http.StatusNotFound
		})
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.created", gone, time.Now(), time.Now()))
		assert.Equal(t, http.StatusUnauthorized, code, body)
		assert.Contains(t, body, "module_inactive")
	})
}

// deactivatePlans starts a test from a module that is not active, whatever an earlier test (they share a
// database) left: it disables the module if it is active, and checks its routes are gone.
func deactivatePlans(ctx context.Context, t *testing.T, w plansWorld) {
	t.Helper()
	_ = w.host.Disable(ctx, aihub.Name) // fails only when the module is not active
	eventually(t, "the module routes to go", func() bool {
		code, _ := w.call(t, w.full, http.MethodGet, plansPath, nil)
		return code == http.StatusNotFound
	})
}

// installPlans installs the ai-hub module, as the distribution's modules list does at boot, and waits
// for its routes.
func installPlans(ctx context.Context, t *testing.T, w plansWorld) {
	t.Helper()
	_, err := w.host.Install(ctx, aihub.Name, false)
	require.NoError(t, err)
	eventually(t, "the module routes", func() bool {
		code, _ := w.call(t, w.full, http.MethodGet, plansPath, nil)
		return code == http.StatusOK
	})
}

func TestSubscriptionPlanGatewaySync(t *testing.T) {
	t.Setenv("MODULE_AI_HUB_WEBHOOK_SECRET", webhookSecret)
	w := newPlansWorld(t)
	ctx := context.Background()
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/projects",
		projectDocument("gwsync", "Gateway sync", nil))
	require.Equal(t, http.StatusAccepted, code, string(body))
	deactivatePlans(ctx, t, w)
	installPlans(ctx, t, w)
	_, err := w.host.Install(ctx, planGatewayModuleName, false)
	require.NoError(t, err)
	eventually(t, "the stand-in gateway", func() bool { _, ok := coreext.Capability(planGatewayReady); return ok })

	sent := func(t *testing.T, handle string, action subplanv1.Action) sentPlan {
		t.Helper()
		var got sentPlan
		eventually(t, string(action)+" sent to the gateways for "+handle, func() bool {
			var ok bool
			got, ok = w.gateways.find(handle, action)
			return ok
		})
		return got
	}
	handle := "gw-" + uuid.NewString()[:8]
	notSent := func(t *testing.T, handle string) {
		t.Helper()
		time.Sleep(500 * time.Millisecond)
		_, ok := w.gateways.find(handle, subplanv1.Created)
		assert.False(t, ok, "a plan reaches the gateways with its subscriptions only")
	}

	t.Run("a plan made or changed here is not pushed", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, planDoc(handle, map[string]any{
			"displayName": "Gateway Gold",
			"conditions": []any{
				map[string]any{"type": "request-count", "params": map[string]any{"limits": []any{
					map[string]any{"count": 5, "timeAmount": 10, "timeUnit": "MINUTE"},
					map[string]any{"count": 1000, "timeAmount": 1, "timeUnit": "HOUR"},
				}}},
				map[string]any{"type": "tool-call-count", "params": map[string]any{"limits": []any{
					map[string]any{"count": 30, "timeAmount": 1, "timeUnit": "MINUTE"},
				}}},
			},
		}))
		require.Equal(t, http.StatusCreated, code, string(body))
		code, body = w.call(t, w.full, http.MethodPut, plansPath+"/"+handle,
			planDoc(handle, map[string]any{"displayName": "Gateway Gold", "description": "changed"}))
		require.Equal(t, http.StatusOK, code, string(body))
		notSent(t, handle)
	})

	t.Run("nor is a plan the developer portal sends by webhook", func(t *testing.T) {
		portal := "portal-" + uuid.NewString()[:8]
		now := time.Now()
		code, body := send(t, w.internal, delivery(t, w.org, "subscription_plan.created", map[string]any{
			"handle": portal, "display_name": "Portal Silver",
		}, now, now))
		require.Equal(t, http.StatusOK, code, body)
		notSent(t, portal)
	})

	t.Run("a subscription brings its plan to the gateways first, with its legacy limit", func(t *testing.T) {
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.created",
			map[string]any{"subscription_plan": map[string]any{"ref_id": handle}}, "key-1", time.Now()))
		require.Equal(t, http.StatusOK, code, body)
		got := sent(t, handle, subplanv1.Created)
		assert.Equal(t, w.org, got.org)
		assert.NotEmpty(t, got.plan.ID, "gateways key plans by the plan's id")
		assert.Equal(t, "Gateway Gold", got.plan.Name)
		assert.Equal(t, "ACTIVE", got.plan.Status)
		require.NotNil(t, got.plan.ThrottleLimitCount)
		assert.Equal(t, 1000, *got.plan.ThrottleLimitCount, "the first window of one unit")
		assert.Equal(t, "HOUR", got.plan.ThrottleLimitUnit)
		assert.True(t, got.plan.StopOnQuotaReach)
		assert.Len(t, got.plan.Conditions, 2)
	})

	t.Run("gateways that connect pull only the plans subscribed to", func(t *testing.T) {
		source, ok := coreext.Lookup[subplanv1.Source](subplanv1.ID)
		require.True(t, ok)
		plans, err := source.ListPlans(ctx, w.org)
		require.NoError(t, err)
		pulled, err := gwsvc.SubscribedPlans(ctx, source, standInDeployments{}, w.org, plans)
		require.NoError(t, err)
		require.Len(t, pulled, 1)
		assert.Equal(t, handle, pulled[0].Handle)
		created, _ := w.gateways.find(handle, subplanv1.Created)
		assert.Equal(t, created.plan.ID, pulled[0].ID, "the same id is pushed and pulled")
	})

	t.Run("a deleted subscription leaves its plan, which is free again", func(t *testing.T) {
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.deleted", nil, "",
			time.Now().Add(time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		_, ok := w.gateways.find(handle, subplanv1.Deleted)
		assert.False(t, ok)
		code, resp := w.call(t, w.full, http.MethodPut, plansPath+"/"+handle,
			planDoc(handle, map[string]any{"displayName": "Gateway Gold renamed"}))
		assert.Equal(t, http.StatusOK, code, string(resp))
	})

	t.Run("without the gateway, plan changes still succeed", func(t *testing.T) {
		require.NoError(t, w.host.Disable(ctx, planGatewayModuleName))
		eventually(t, "the stand-in gateway to go", func() bool { _, ok := coreext.Capability(planGatewayReady); return !ok })
		lone := "lone-" + uuid.NewString()[:8]
		code, body := w.call(t, w.full, http.MethodPost, plansPath, planDoc(lone, map[string]any{"displayName": "Lone"}))
		require.Equal(t, http.StatusCreated, code, string(body))
		time.Sleep(500 * time.Millisecond)
		_, ok := w.gateways.find(lone, subplanv1.Created)
		assert.False(t, ok, "nothing to send it to")
	})
}

// subscriptionDelivery is a signed subscription event for the MCP proxy docs-mcp, as the portal sends it. A
// token, when given, is sealed with the webhook secret.
func subscriptionDelivery(t *testing.T, org, eventType string, data map[string]any, token string, occurredAt time.Time) *http.Request {
	t.Helper()
	d := map[string]any{
		"subscription_id": "portal-sub-1", "subscriber_id": "alice", "status": "ACTIVE",
		"api": map[string]any{"ref_id": "docs-mcp", "type": "Mcp", "name": "Docs MCP", "version": "v1.0"},
	}
	for k, v := range data {
		d[k] = v
	}
	if token != "" {
		sealed, err := aihub.SealField(webhookSecret, token, []byte(uuid.NewString()[:12]))
		require.NoError(t, err)
		d["token"] = sealed
	}
	return delivery(t, org, eventType, d, occurredAt, time.Now())
}

// TestSubscriptionsFromThePortal copies subscriptions in from signed portal deliveries, and checks what gateways
// would get: the subscription with its key, through subscription.plans/v1, and an event on every change.
func TestSubscriptionsFromThePortal(t *testing.T) {
	t.Setenv("MODULE_AI_HUB_WEBHOOK_SECRET", webhookSecret)
	w := newPlansWorld(t)
	ctx := context.Background()
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/projects",
		projectDocument("subs", "Subscriptions", nil))
	require.Equal(t, http.StatusAccepted, code, string(body))
	deactivatePlans(ctx, t, w)
	installPlans(ctx, t, w)
	_, err := w.host.Install(ctx, planGatewayModuleName, false)
	require.NoError(t, err)
	eventually(t, "the stand-in gateway", func() bool { _, ok := coreext.Capability(planGatewayReady); return ok })

	gold, silver := "sub-gold-"+uuid.NewString()[:6], "sub-silver-"+uuid.NewString()[:6]
	for _, id := range []string{gold, silver} {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, planDoc(id, map[string]any{
			"displayName": id,
			"conditions": []any{map[string]any{"type": "tool-call-count", "params": map[string]any{"limits": []any{
				map[string]any{"count": 3, "timeAmount": 1, "timeUnit": "MINUTE"},
			}}}},
		}))
		require.Equal(t, http.StatusCreated, code, string(body))
	}
	planRef := func(h string) map[string]any { return map[string]any{"subscription_plan": map[string]any{"ref_id": h}} }
	list := func(t *testing.T) []subplanv1.Subscription {
		t.Helper()
		source, ok := coreext.Lookup[subplanv1.Source](subplanv1.ID)
		require.True(t, ok)
		subs, err := source.ListSubscriptions(ctx, w.org, subplanv1.ArtifactKindMCP, "docs-mcp")
		require.NoError(t, err)
		return subs
	}
	usageOf := func(t *testing.T, h string) planBody {
		t.Helper()
		code, body := w.call(t, w.full, http.MethodGet, plansPath+"/"+h, nil)
		require.Equal(t, http.StatusOK, code, string(body))
		return decodeAs[planBody](t, body)
	}
	now := time.Now()
	var subID string

	t.Run("created: stored with its key, published, and its plan in use", func(t *testing.T) {
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.created", planRef(gold), "key-1", now))
		require.Equal(t, http.StatusOK, code, body)
		subs := list(t)
		require.Len(t, subs, 1)
		subID = subs[0].ID
		assert.Equal(t, "key-1", subs[0].Token)
		assert.Equal(t, gold, subs[0].PlanHandle)
		assert.Equal(t, subplanv1.SubscriptionActive, subs[0].Status)
		assert.Equal(t, "alice", subs[0].SubscriberID)
		eventually(t, "the created change", func() bool { _, ok := w.gateways.subscriptionChange(subID, subplanv1.Created); return ok })

		p := usageOf(t, gold)
		require.NotNil(t, p.Usage)
		assert.True(t, p.Usage.InUse)
		for name, spec := range map[string]map[string]any{
			"name":       {"displayName": "Renamed"},
			"conditions": {"displayName": gold, "conditions": []any{}},
			"expiry":     {"displayName": gold, "expiryTime": "2099-01-31T23:59:59Z"},
		} {
			code, resp := w.call(t, w.full, http.MethodPut, plansPath+"/"+gold, planDoc(gold, spec))
			assert.Equal(t, http.StatusConflict, code, "a subscribed plan keeps its %s: %s", name, resp)
			assert.Equal(t, "SUBSCRIPTION_PLAN_IN_USE", decodeAs[errBody](t, resp).Code, name)
		}
		code, resp := w.call(t, w.full, http.MethodPut, plansPath+"/"+gold,
			planDoc(gold, map[string]any{"displayName": gold, "description": "still editable", "status": "INACTIVE"}))
		assert.Equal(t, http.StatusOK, code, "its description and status can change: "+string(resp))
		code, resp = w.call(t, w.full, http.MethodPut, plansPath+"/"+gold,
			planDoc(gold, map[string]any{"displayName": gold, "status": "ACTIVE"}))
		require.Equal(t, http.StatusOK, code, string(resp))
	})

	t.Run("a repeated creation changes nothing", func(t *testing.T) {
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.created", planRef(gold), "key-1", now))
		require.Equal(t, http.StatusOK, code, body)
		assert.Len(t, list(t), 1)
	})

	t.Run("token regenerated, plan changed and deactivated follow; each is published", func(t *testing.T) {
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.token_regenerated", nil, "key-2", now.Add(time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.plan_changed", planRef(silver), "", now.Add(2*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.updated", map[string]any{"status": "INACTIVE"}, "", now.Add(3*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		subs := list(t)
		require.Len(t, subs, 1)
		assert.Equal(t, "key-2", subs[0].Token)
		assert.Equal(t, silver, subs[0].PlanHandle)
		assert.Equal(t, subplanv1.SubscriptionInactive, subs[0].Status)
		eventually(t, "an updated change", func() bool { _, ok := w.gateways.subscriptionChange(subID, subplanv1.Updated); return ok })
	})

	t.Run("an older event changes nothing", func(t *testing.T) {
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.updated", map[string]any{"status": "ACTIVE"}, "", now.Add(time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, subplanv1.SubscriptionInactive, list(t)[0].Status)
	})

	t.Run("a deprecated plan keeps its subscriptions, and gateways still get it, but it takes no new ones", func(t *testing.T) {
		code, resp := w.call(t, w.full, http.MethodDelete, plansPath+"/"+silver, nil)
		require.Equal(t, http.StatusOK, code, "a subscribed plan can be deprecated: "+string(resp))

		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.updated", map[string]any{"status": "ACTIVE"}, "", now.Add(5*time.Second)))
		require.Equal(t, http.StatusOK, code, "a subscription already on it still changes: "+body)
		subs := list(t)
		require.Len(t, subs, 1)
		assert.Equal(t, silver, subs[0].PlanHandle)
		assert.Equal(t, subplanv1.SubscriptionActive, subs[0].Status)

		source, ok := coreext.Lookup[subplanv1.Source](subplanv1.ID)
		require.True(t, ok)
		plans, err := source.ListPlans(ctx, w.org)
		require.NoError(t, err)
		pulled, err := gwsvc.SubscribedPlans(ctx, source, standInDeployments{}, w.org, plans)
		require.NoError(t, err)
		require.Len(t, pulled, 1, "gateways that connect still pull it")
		assert.Equal(t, silver, pulled[0].Handle)
		assert.True(t, pulled[0].Deprecated)

		bob := subscriptionDelivery(t, w.org, "subscription.created", map[string]any{
			"subscription_id": "portal-sub-4", "subscriber_id": "bob", "subscription_plan": map[string]any{"ref_id": silver},
		}, "key-5", now)
		code, body = send(t, w.internal, bob)
		assert.Equal(t, http.StatusConflict, code, "a new subscription is refused, and not retried: "+body)
		assert.Contains(t, body, "SUBSCRIPTION_PLAN_DEPRECATED")
		assert.Len(t, list(t), 1)

		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.plan_changed", planRef(gold), "", now.Add(6*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.plan_changed", planRef(silver), "", now.Add(7*time.Second)))
		assert.Equal(t, http.StatusConflict, code, "nor can a subscription move onto it: "+body)
		assert.Equal(t, gold, list(t)[0].PlanHandle)
	})

	t.Run("a subscription on a plan that is not here, or to a REST API, is refused", func(t *testing.T) {
		other := subscriptionDelivery(t, w.org, "subscription.created", map[string]any{
			"subscription_id": "portal-sub-2", "subscriber_id": "bob", "subscription_plan": map[string]any{"ref_id": "no-such-plan"},
		}, "key-3", now)
		code, _ := send(t, w.internal, other)
		assert.Equal(t, http.StatusUnauthorized, code)
		rest := subscriptionDelivery(t, w.org, "subscription.created", map[string]any{
			"subscription_id": "portal-sub-3", "subscriber_id": "bob", "subscription_plan": map[string]any{"ref_id": gold},
			"api": map[string]any{"ref_id": "orders", "type": "RestApi"},
		}, "key-4", now)
		code, _ = send(t, w.internal, rest)
		assert.Equal(t, http.StatusUnauthorized, code)
		assert.Len(t, list(t), 1)
	})

	t.Run("deleted: gone with its key, published, and its plan free again", func(t *testing.T) {
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.deleted", nil, "", now.Add(8*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		assert.Empty(t, list(t))
		eventually(t, "the deleted change", func() bool { _, ok := w.gateways.subscriptionChange(subID, subplanv1.Deleted); return ok })
		assert.False(t, usageOf(t, gold).Usage.InUse)
		code, resp := w.call(t, w.full, http.MethodPut, plansPath+"/"+gold, planDoc(gold, map[string]any{"displayName": "Gold again"}))
		assert.Equal(t, http.StatusOK, code, string(resp))
	})

	t.Run("subscribing again replaces the old subscription, which a late deletion then doesn't touch", func(t *testing.T) {
		as := func(sourceID string) map[string]any {
			return map[string]any{"subscription_id": sourceID, "subscription_plan": map[string]any{"ref_id": gold}}
		}
		code, body := send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.created", as("portal-sub-6"), "key-6", now.Add(10*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.created", as("portal-sub-7"), "key-7", now.Add(12*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		subs := list(t)
		require.Len(t, subs, 1, "one subscription per subscriber and MCP server")
		assert.Equal(t, "key-7", subs[0].Token, "the newer subscription replaced the older one")

		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.deleted", as("portal-sub-6"), "", now.Add(11*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.created", as("portal-sub-5"), "key-5", now.Add(9*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		subs = list(t)
		require.Len(t, subs, 1, "the old deletion and an older creation change nothing")
		assert.Equal(t, "key-7", subs[0].Token)

		code, body = send(t, w.internal, subscriptionDelivery(t, w.org, "subscription.deleted", as("portal-sub-7"), "", now.Add(13*time.Second)))
		require.Equal(t, http.StatusOK, code, body)
		assert.Empty(t, list(t))
	})
}

// TestPlanOfferedByMCPProxies counts the MCP proxies the gateway reports as offering a plan
// (subplanv1.Source.CountOffersWith) in the plan's usage. An offer doesn't lock the plan: only subscriptions do.
func TestPlanOfferedByMCPProxies(t *testing.T) {
	w := newPlansWorld(t)
	ctx := context.Background()
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/projects",
		projectDocument("offers", "Offers", nil))
	require.Equal(t, http.StatusAccepted, code, string(body))
	deactivatePlans(ctx, t, w)
	installPlans(ctx, t, w)
	_ = w.host.Disable(ctx, planGatewayModuleName) // an earlier test may have left the stand-in gateway on
	eventually(t, "the stand-in gateway to go", func() bool { _, ok := coreext.Capability(planGatewayReady); return !ok })
	handle := "offered-" + uuid.NewString()[:6]
	code, body = w.call(t, w.full, http.MethodPost, plansPath, planDoc(handle, map[string]any{"displayName": "Offered"}))
	require.Equal(t, http.StatusCreated, code, string(body))
	usageOf := func(t *testing.T) planBody {
		t.Helper()
		code, body := w.call(t, w.full, http.MethodGet, plansPath+"/"+handle, nil)
		require.Equal(t, http.StatusOK, code, string(body))
		return decodeAs[planBody](t, body)
	}

	t.Run("without the gateway nothing offers it", func(t *testing.T) {
		w.gateways.setOffers(handle, 2)
		assert.Zero(t, usageOf(t).Usage.MCPProxies)
	})

	if _, err := w.host.Install(ctx, planGatewayModuleName, false); err != nil {
		require.NoError(t, w.host.Enable(ctx, planGatewayModuleName))
	}
	eventually(t, "the stand-in gateway", func() bool { _, ok := coreext.Capability(planGatewayReady); return ok })

	t.Run("offered by MCP proxies, the plan is counted, and still editable", func(t *testing.T) {
		p := usageOf(t)
		assert.Equal(t, 2, p.Usage.MCPProxies)
		assert.False(t, p.Usage.InUse, "an offer alone doesn't lock the plan")
		code, body := w.call(t, w.full, http.MethodPut, plansPath+"/"+handle, planDoc(handle, map[string]any{
			"displayName": "Renamed", "expiryTime": "2099-01-31T23:59:59Z",
			"conditions": []any{map[string]any{"type": "tool-call-count", "params": map[string]any{
				"limits": []any{map[string]any{"count": 5, "timeUnit": "MINUTE"}},
			}}},
		}))
		assert.Equal(t, http.StatusOK, code, "its name, expiry and conditions can change: "+string(body))
	})

	t.Run("an offered plan can be deprecated, and is then read only", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodDelete, plansPath+"/"+handle, nil)
		require.Equal(t, http.StatusOK, code, string(body))
		p := usageOf(t)
		assert.True(t, p.Deprecated)
		assert.Equal(t, 2, p.Usage.MCPProxies, "the offers stay")
		code, body = w.call(t, w.full, http.MethodPut, plansPath+"/"+handle, planDoc(handle, map[string]any{"displayName": "Again"}))
		assert.Equal(t, http.StatusConflict, code, string(body))
		w.gateways.setOffers(handle, 0)
	})
}
