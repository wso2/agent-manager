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

const plansPath = "/m/aihub/subscription-plans"

type plansWorld struct {
	moduleWorld
	reader      http.Handler // may read plans only
	other       http.Handler // another organization, with every plan permission
	otherHandle string
	gateways    *planGateways // what the stand-in gateway module was sent, once installed
}

// planGateways records the plan changes the gateway handler would send to the organization's gateways.
type planGateways struct {
	mu   sync.Mutex
	sent []sentPlan
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

// planGatewayModule stands in for the AI Gateway: it subscribes to the plans module's event with the gateway's
// own handler (gwsvc.PlanChangeHandler) and records what it would send to the organization's gateways. It
// provides planGatewayReady so a test can wait for it.
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
	rt.Subscribe(subplanv1.EventPlanChanged, gwsvc.PlanChangeHandler(func() (subplanv1.Source, bool) {
		return modkit.Lookup[subplanv1.Source](rt, subplanv1.ID)
	}, m.gateways))
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
	full := params(moduleAuth(org, handle, "amp:aihub:plan-read", "amp:aihub:plan-create", "amp:aihub:plan-update", "amp:aihub:plan-delete"))
	reader := params(moduleAuth(org, handle, "amp:aihub:plan-read"))
	limited := params(moduleAuth(org, handle))
	other := params(moduleAuth(otherOrg, otherHandle, "amp:aihub:plan-read", "amp:aihub:plan-create", "amp:aihub:plan-update", "amp:aihub:plan-delete"))

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

type planBody struct {
	ID             string            `json:"id"`
	DisplayName    string            `json:"displayName"`
	Description    string            `json:"description"`
	Conditions     []json.RawMessage `json:"conditions"`
	Limits         []map[string]any  `json:"limits"`
	ExpiryTime     *time.Time        `json:"expiryTime"`
	OrganizationID string            `json:"organizationId"`
	Status         string            `json:"status"`
	CreatedBy      string            `json:"createdBy"`
	UpdatedBy      *string           `json:"updatedBy"`
	Usage          *struct {
		InUse         bool `json:"inUse"`
		MCPProxies    int  `json:"mcpProxies"`
		Subscriptions int  `json:"subscriptions"`
	} `json:"usage"`
}

type errBody struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeAs[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(body, &v), string(body))
	return v
}

// TestSubscriptionPlansModule drives the subscription plans module's API
// (/subscription-plans) through the core's module host, sign-in, permission checks and audit. The
// module is built in, so it is active without an install.
func TestSubscriptionPlansModule(t *testing.T) {
	w := newPlansWorld(t)
	ctx := context.Background()
	gold := map[string]any{
		"id": "gold", "displayName": "Gold", "description": "For production agents",
		"conditions": []any{
			map[string]any{"type": "request-count", "params": map[string]any{"limits": []any{
				map[string]any{"count": 1000, "timeUnit": "HOUR"},
			}}},
			map[string]any{"type": "tool-call-count", "params": map[string]any{"limits": []any{
				map[string]any{"count": 3, "timeAmount": 1, "timeUnit": "MINUTE"},
			}, "onExceed": "reject"}},
		},
	}

	// Creating a project creates the organization in the core: plans need their organization to exist.
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/orgs/"+w.handle+"/projects",
		map[string]any{"name": "plans", "displayName": "Plans"})
	require.Equal(t, http.StatusAccepted, code, string(body))
	code, body = w.call(t, w.other, http.MethodPost, "/api/v1/orgs/"+w.otherHandle+"/projects",
		map[string]any{"name": "plans", "displayName": "Plans"})
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

	t.Run("create fills defaults and returns the full plan", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, gold)
		require.Equal(t, http.StatusCreated, code, string(body))
		p := decodeAs[planBody](t, body)
		assert.Equal(t, "gold", p.ID)
		assert.Equal(t, "ACTIVE", p.Status)
		assert.Equal(t, w.handle, p.OrganizationID)
		assert.Equal(t, "module-tester", p.CreatedBy)
		require.Len(t, p.Conditions, 2)
		assert.JSONEq(t, `{"id":"requests","type":"request-count","params":{"limits":[{"count":1000,"timeAmount":1,"timeUnit":"HOUR"}],"onExceed":"reject"}}`,
			string(p.Conditions[0]))
		require.Len(t, p.Limits, 1, "the old limits field mirrors the request-count condition")
		assert.EqualValues(t, 1000, p.Limits[0]["limitCount"])
	})

	t.Run("create refuses a duplicate, bad input and pricing", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, gold)
		assert.Equal(t, http.StatusConflict, code)
		assert.Equal(t, "SUBSCRIPTION_PLAN_EXISTS", decodeAs[errBody](t, body).Code)

		for name, b := range map[string]map[string]any{
			"no id":                  {"displayName": "X"},
			"long id":                {"id": "a234567890123456789012345678901234567890x", "displayName": "X"},
			"no name":                {"id": "x"},
			"bad status":             {"id": "x", "displayName": "X", "status": "DRAFT"},
			"conditions and limits":  {"id": "x", "displayName": "X", "conditions": []any{}, "limits": []any{}},
			"two request conditions": {"id": "x", "displayName": "X", "conditions": []any{gold["conditions"].([]any)[0], gold["conditions"].([]any)[0]}},
			"bad expiry":             {"id": "x", "displayName": "X", "expiryTime": "tomorrow"},
			"pricing":                {"id": "x", "displayName": "X", "pricing": map[string]any{"pricingType": "FLAT_RATE"}},
		} {
			code, body := w.call(t, w.full, http.MethodPost, plansPath, b)
			assert.Equal(t, http.StatusBadRequest, code, name)
			e := decodeAs[errBody](t, body)
			assert.Equal(t, "VALIDATION_FAILED", e.Code, name)
			assert.Equal(t, "error", e.Status, name)
		}
	})

	t.Run("old limits become a request-count condition", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, map[string]any{
			"id": "legacy", "displayName": "Legacy", "status": "INACTIVE",
			"limits": []any{map[string]any{"timeUnit": "MINUTE", "limitCount": 10, "stopOnQuotaReach": false}},
		})
		require.Equal(t, http.StatusCreated, code, string(body))
		p := decodeAs[planBody](t, body)
		require.Len(t, p.Conditions, 1)
		assert.JSONEq(t, `{"id":"requests","type":"request-count","params":{"limits":[{"count":10,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"allow"}}`,
			string(p.Conditions[0]))
	})

	t.Run("get carries usage and updatedBy", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, plansPath+"/gold", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		p := decodeAs[planBody](t, body)
		require.NotNil(t, p.Usage)
		assert.False(t, p.Usage.InUse)
		require.NotNil(t, p.UpdatedBy)
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
		assert.Equal(t, 2, list.Count)
		assert.Equal(t, 2, list.Pagination.Total)
		assert.Equal(t, 20, list.Pagination.Limit)
		assert.Equal(t, "legacy", list.List[0].ID, "newest first")
		assert.Nil(t, list.List[0].UpdatedBy, "list items leave out updatedBy")
		assert.Nil(t, list.List[0].Usage, "list items leave out usage")

		_, body = w.call(t, w.full, http.MethodGet, plansPath+"?query=GOL", nil)
		require.NoError(t, json.Unmarshal(body, &list))
		require.Equal(t, 1, list.Count)
		assert.Equal(t, "gold", list.List[0].ID)

		_, body = w.call(t, w.full, http.MethodGet, plansPath+"?query=%25", nil)
		require.NoError(t, json.Unmarshal(body, &list))
		assert.Equal(t, 0, list.Count, "% is taken literally")

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
		code, body := w.call(t, w.full, http.MethodPut, plansPath+"/gold", map[string]any{
			"displayName": "Gold+", "expiryTime": expiry, "status": "INACTIVE",
		})
		require.Equal(t, http.StatusOK, code, string(body))
		p := decodeAs[planBody](t, body)
		assert.Equal(t, "Gold+", p.DisplayName)
		assert.Equal(t, "For production agents", p.Description, "an omitted description is kept")
		assert.Len(t, p.Conditions, 2, "omitted conditions are kept")
		require.NotNil(t, p.ExpiryTime)
		assert.Equal(t, "INACTIVE", p.Status)

		code, body = w.call(t, w.full, http.MethodPut, plansPath+"/gold", map[string]any{
			"displayName": "Gold+", "expiryTime": nil, "conditions": []any{},
		})
		require.Equal(t, http.StatusOK, code, string(body))
		p = decodeAs[planBody](t, body)
		assert.Nil(t, p.ExpiryTime)
		assert.Empty(t, p.Conditions, "[] is a plan with no conditions")
		assert.Equal(t, "INACTIVE", p.Status, "an omitted status is kept")

		for name, b := range map[string]map[string]any{
			"no name":       {"description": "x"},
			"id change":     {"id": "silver", "displayName": "Gold"},
			"pricing":       {"displayName": "Gold", "pricing": nil},
			"null conds":    {"displayName": "Gold", "conditions": nil},
			"bad condition": {"displayName": "Gold", "conditions": []any{map[string]any{"type": "event-count"}}},
		} {
			code, _ := w.call(t, w.full, http.MethodPut, plansPath+"/gold", b)
			assert.Equal(t, http.StatusBadRequest, code, name)
		}
		code, _ = w.call(t, w.full, http.MethodPut, plansPath+"/missing", map[string]any{"displayName": "X"})
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

	t.Run("delete removes the plan and its conditions", func(t *testing.T) {
		code, _ := w.call(t, w.full, http.MethodDelete, plansPath+"/legacy", nil)
		assert.Equal(t, http.StatusNoContent, code)
		code, _ = w.call(t, w.full, http.MethodGet, plansPath+"/legacy", nil)
		assert.Equal(t, http.StatusNotFound, code)
		var left int64
		require.NoError(t, db.DB(ctx).Raw(`SELECT COUNT(*) FROM mod_aihub_subscription_plan_conditions c `+
			`WHERE NOT EXISTS (SELECT 1 FROM mod_aihub_subscription_plans p WHERE p.id = c.plan_id)`).Scan(&left).Error)
		assert.Zero(t, left)
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
	t.Setenv("MODULE_AIHUB_WEBHOOK_SECRET", webhookSecret)
	w := newPlansWorld(t)
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/orgs/"+w.handle+"/projects",
		map[string]any{"name": "hooks", "displayName": "Hooks"})
	require.Equal(t, http.StatusAccepted, code, body)
	now := time.Now()
	gone := map[string]any{"handle": "never-copied", "display_name": "Never copied"}
	deactivatePlans(context.Background(), t, w)
	code, refused := send(t, w.internal, delivery(t, w.handle, "subscription_plan.created", gone, now, now))
	// The host serves a module's fixed internal route only while the module is active.
	require.Equal(t, http.StatusNotFound, code, "a module that isn't active answers no delivery: "+refused)
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
		code, body := send(t, w.internal, delivery(t, w.handle, "subscription_plan.created", mcpPlan, made, now))
		require.Equal(t, http.StatusOK, code, body)
		assert.JSONEq(t, `{"status":"processed"}`, body)
		code, got := w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		require.Equal(t, http.StatusOK, code, string(got))
		p := decodeAs[planBody](t, got)
		assert.Equal(t, "MCP Gold", p.DisplayName)
		assert.Equal(t, "ACTIVE", p.Status)
		require.Len(t, p.Conditions, 1)
		assert.JSONEq(t, `{"id":"tool-calls","type":"tool-call-count","params":{"limits":[{"count":30,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"reject"}}`,
			string(p.Conditions[0]))
	})

	t.Run("a repeated delivery changes nothing", func(t *testing.T) {
		code, body := send(t, w.internal, delivery(t, w.handle, "subscription_plan.created", mcpPlan, now, now))
		require.Equal(t, http.StatusOK, code, body)
		_, got := w.call(t, w.full, http.MethodGet, plansPath+"?query=mcp-gold", nil)
		assert.Contains(t, string(got), `"count":1`)
	})

	t.Run("updated: name and conditions follow; old senders' limits change only requests", func(t *testing.T) {
		upd := map[string]any{
			"handle": "mcp-gold", "display_name": "MCP Gold+",
			"limits": []any{map[string]any{"limit_type": "REQUEST_COUNT", "limit_count": 100, "time_unit": "MINUTE", "time_amount": 1}},
		}
		code, body := send(t, w.internal, delivery(t, w.handle, "subscription_plan.updated", upd, now, now))
		require.Equal(t, http.StatusOK, code, body)
		_, got := w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		p := decodeAs[planBody](t, got)
		assert.Equal(t, "MCP Gold+", p.DisplayName)
		require.Len(t, p.Conditions, 2, "the request limit is added; the tool-call condition stays")
	})

	t.Run("refused alike: bad signature, stale signature, unknown org, unknown event", func(t *testing.T) {
		bad := delivery(t, w.handle, "subscription_plan.created", mcpPlan, now, now)
		bad.Header.Set(aihub.DefaultWebhookSignatureHeader, aihub.Sign("wrong-secret", []byte("x"), now))
		stale := delivery(t, w.handle, "subscription_plan.created", mcpPlan, now, now.Add(-10*time.Minute))
		unknownOrg := delivery(t, "no-such-org", "subscription_plan.created", mcpPlan, now, now)
		unknownEvent := delivery(t, w.handle, "meter.created", map[string]any{"handle": "m"}, now, now)
		for name, req := range map[string]*http.Request{
			"bad signature": bad, "stale signature": stale, "unknown org": unknownOrg,
			"unknown event": unknownEvent,
		} {
			code, body := send(t, w.internal, req)
			assert.Equal(t, http.StatusUnauthorized, code, name)
			assert.JSONEq(t, `{"status":"error","code":"UNAUTHORIZED","message":"Invalid or expired credentials."}`, body, name)
		}
	})

	t.Run("a priced plan is stored without its price", func(t *testing.T) {
		paid := map[string]any{
			"handle": "paid", "display_name": "Paid", "pricing": map[string]any{"pricingType": "FLAT_RATE"},
			"conditions": []any{map[string]any{"type": "request-count", "params": map[string]any{
				"limits": []any{map[string]any{"count": 10, "timeAmount": 1, "timeUnit": "MINUTE"}},
			}}},
		}
		code, body := send(t, w.internal, delivery(t, w.handle, "subscription_plan.created", paid, now, now))
		require.Equal(t, http.StatusOK, code, body)
		code, got := w.call(t, w.full, http.MethodGet, plansPath+"/paid", nil)
		require.Equal(t, http.StatusOK, code, string(got))
		p := decodeAs[planBody](t, got)
		assert.Equal(t, "Paid", p.DisplayName)
		assert.Len(t, p.Conditions, 1, "its conditions are kept")
		assert.NotContains(t, string(got), "pricing")
	})

	t.Run("deleted: an older deletion keeps a newer plan; a current one deletes; a repeat succeeds", func(t *testing.T) {
		del := map[string]any{"handle": "mcp-gold"}
		code, body := send(t, w.internal, delivery(t, w.handle, "subscription_plan.deleted", del, now.Add(-2*time.Hour), now))
		require.Equal(t, http.StatusOK, code, body)
		code, _ = w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		assert.Equal(t, http.StatusOK, code, "the plan was made after that deletion")

		code, body = send(t, w.internal, delivery(t, w.handle, "subscription_plan.deleted", del, now, now))
		require.Equal(t, http.StatusOK, code, body)
		code, _ = w.call(t, w.full, http.MethodGet, plansPath+"/mcp-gold", nil)
		assert.Equal(t, http.StatusNotFound, code)

		code, body = send(t, w.internal, delivery(t, w.handle, "subscription_plan.deleted", del, now, now))
		assert.Equal(t, http.StatusOK, code, "a plan already gone counts as deleted: "+body)
	})

	t.Run("disabled: neither the API nor the webhook answers", func(t *testing.T) {
		require.NoError(t, w.host.Disable(context.Background(), aihub.Name))
		eventually(t, "the module routes to go", func() bool {
			code, _ := w.call(t, w.full, http.MethodGet, plansPath, nil)
			return code == http.StatusNotFound
		})
		code, body := send(t, w.internal, delivery(t, w.handle, "subscription_plan.created", gone, time.Now(), time.Now()))
		assert.Equal(t, http.StatusNotFound, code, body)
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

// installPlans installs the aihub module, as the distribution's modules list does at boot, and waits
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
	t.Setenv("MODULE_AIHUB_WEBHOOK_SECRET", webhookSecret)
	w := newPlansWorld(t)
	ctx := context.Background()
	code, body := w.call(t, w.full, http.MethodPost, "/api/v1/orgs/"+w.handle+"/projects",
		map[string]any{"name": "gwsync", "displayName": "Gateway sync"})
	require.Equal(t, http.StatusAccepted, code, string(body))
	deactivatePlans(ctx, t, w)
	installPlans(ctx, t, w)
	_, err := w.host.Install(ctx, planGatewayModuleName, false)
	require.NoError(t, err)
	eventually(t, "the stand-in gateway", func() bool { _, ok := coreext.Capability(planGatewayReady); return ok })

	sent := func(handle string, action subplanv1.Action) sentPlan {
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

	t.Run("a plan made here is pushed to the organization's gateways, with its legacy limit", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, plansPath, map[string]any{
			"id": handle, "displayName": "Gateway Gold",
			"conditions": []any{
				map[string]any{"type": "request-count", "params": map[string]any{"limits": []any{
					map[string]any{"count": 5, "timeAmount": 10, "timeUnit": "MINUTE"},
					map[string]any{"count": 1000, "timeAmount": 1, "timeUnit": "HOUR"},
				}}},
				map[string]any{"type": "tool-call-count", "params": map[string]any{"limits": []any{
					map[string]any{"count": 30, "timeAmount": 1, "timeUnit": "MINUTE"},
				}}},
			},
		})
		require.Equal(t, http.StatusCreated, code, string(body))
		got := sent(handle, subplanv1.Created)
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

	t.Run("gateways that connect pull every plan of their organization", func(t *testing.T) {
		source, ok := coreext.Lookup[subplanv1.Source](subplanv1.ID)
		require.True(t, ok)
		plans, err := source.ListPlans(ctx, w.org)
		require.NoError(t, err)
		var found *subplanv1.Plan
		for i := range plans {
			if plans[i].Handle == handle {
				found = &plans[i]
			}
		}
		require.NotNil(t, found)
		created, _ := w.gateways.find(handle, subplanv1.Created)
		assert.Equal(t, created.plan.ID, found.ID, "the same id is pushed and pulled")
		assert.False(t, found.UpdatedAt.IsZero())
	})

	t.Run("a change and a deletion are pushed too", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPut, plansPath+"/"+handle, map[string]any{
			"displayName": "Gateway Gold v2", "status": "INACTIVE",
		})
		require.Equal(t, http.StatusOK, code, string(body))
		got := sent(handle, subplanv1.Updated)
		assert.Equal(t, "Gateway Gold v2", got.plan.Name)
		assert.Equal(t, "INACTIVE", got.plan.Status)

		code, _ = w.call(t, w.full, http.MethodDelete, plansPath+"/"+handle, nil)
		require.Equal(t, http.StatusNoContent, code)
		deleted := sent(handle, subplanv1.Deleted)
		created, _ := w.gateways.find(handle, subplanv1.Created)
		assert.Equal(t, created.plan.ID, deleted.plan.ID)
	})

	t.Run("a plan the developer portal sends by webhook reaches the gateways", func(t *testing.T) {
		portal := "portal-" + uuid.NewString()[:8]
		now := time.Now()
		code, body := send(t, w.internal, delivery(t, w.handle, "subscription_plan.created", map[string]any{
			"handle": portal, "display_name": "Portal Silver",
			"conditions": []any{map[string]any{"type": "request-count", "params": map[string]any{
				"limits": []any{map[string]any{"count": 60, "timeAmount": 1, "timeUnit": "MINUTE"}},
			}}},
		}, now, now))
		require.Equal(t, http.StatusOK, code, body)
		got := sent(portal, subplanv1.Created)
		assert.Equal(t, "Portal Silver", got.plan.Name)
		require.NotNil(t, got.plan.ThrottleLimitCount)
		assert.Equal(t, 60, *got.plan.ThrottleLimitCount)
		assert.Equal(t, "MINUTE", got.plan.ThrottleLimitUnit)

		code, body = send(t, w.internal, delivery(t, w.handle, "subscription_plan.deleted",
			map[string]any{"handle": portal}, time.Now(), time.Now()))
		require.Equal(t, http.StatusOK, code, body)
		sent(portal, subplanv1.Deleted)
	})

	t.Run("without the gateway, plan changes still succeed", func(t *testing.T) {
		require.NoError(t, w.host.Disable(ctx, planGatewayModuleName))
		eventually(t, "the stand-in gateway to go", func() bool { _, ok := coreext.Capability(planGatewayReady); return !ok })
		lone := "lone-" + uuid.NewString()[:8]
		code, body := w.call(t, w.full, http.MethodPost, plansPath, map[string]any{"id": lone, "displayName": "Lone"})
		require.Equal(t, http.StatusCreated, code, string(body))
		time.Sleep(500 * time.Millisecond)
		_, ok := w.gateways.find(lone, subplanv1.Created)
		assert.False(t, ok, "nothing to send it to")
	})
}
