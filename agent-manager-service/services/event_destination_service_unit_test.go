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

package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var testEventKey = []byte("0123456789abcdef0123456789abcdef")

// fakeDestinationDriver is a "fake" kind: settings {"dest": "..."} and an
// optional generated secret.
type fakeDestinationDriver struct {
	mu        sync.Mutex
	hasSecret bool
	result    eventdestinations.Result
	sent      []eventdestinations.Destination
	msgs      []eventdestinations.Message
}

func (f *fakeDestinationDriver) Kind() string { return "fake" }

func (f *fakeDestinationDriver) NormalizeConfig(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var cfg struct {
		Dest string `json:"dest"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Dest == "" {
		return nil, utils.ErrInvalidInput
	}
	out, _ := json.Marshal(cfg)
	return out, nil
}

func (f *fakeDestinationDriver) RedactConfig(json.RawMessage) json.RawMessage {
	return json.RawMessage(`{"dest":"masked"}`)
}

func (f *fakeDestinationDriver) Origin(json.RawMessage) string { return "fake://dest" }

func (f *fakeDestinationDriver) NewSecret() (string, bool, error) {
	if !f.hasSecret {
		return "", false, nil
	}
	return "secret-" + uuid.NewString(), true, nil
}

func (f *fakeDestinationDriver) Deliver(
	_ context.Context, d eventdestinations.Destination, m eventdestinations.Message,
) eventdestinations.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, d)
	f.msgs = append(f.msgs, m)
	return f.result
}

func (f *fakeDestinationDriver) sends() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

// credentialDriver is a "cred" kind: settings {"dest": "...", "token": "..."},
// where the token is a credential the user supplies.
type credentialDriver struct{ fakeDestinationDriver }

func (c *credentialDriver) Kind() string { return "cred" }

func (c *credentialDriver) NormalizeConfig(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var cfg struct {
		Dest  string `json:"dest"`
		Token string `json:"token,omitempty"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Dest == "" {
		return nil, utils.ErrInvalidInput
	}
	out, _ := json.Marshal(cfg)
	return out, nil
}

func (c *credentialDriver) SplitCredential(raw json.RawMessage) (json.RawMessage, string) {
	var cfg struct {
		Dest  string `json:"dest"`
		Token string `json:"token"`
	}
	_ = json.Unmarshal(raw, &cfg)
	out, _ := json.Marshal(map[string]string{"dest": cfg.Dest})
	return out, cfg.Token
}

// recordingAuditor keeps every audit event.
type recordingAuditor struct {
	mu     sync.Mutex
	events []audit.Event
	fail   error
}

func (r *recordingAuditor) Record(_ context.Context, e audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recordingAuditor) RecordSync(_ context.Context, e audit.Event) error {
	if r.fail != nil {
		return r.fail
	}
	r.Record(context.Background(), e)
	return nil
}

func (r *recordingAuditor) Close(context.Context) error { return nil }

func (r *recordingAuditor) actions() []audit.Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Action, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.Action)
	}
	return out
}

func auditCtx(r *recordingAuditor) context.Context {
	return audit.WithRecorder(context.Background(), r)
}

// memEventStore is an in-memory EventRepository with the rules the database
// enforces: destination names unique per org ignoring case, and a
// destination that subscriptions use cannot be deleted.
type memEventStore struct {
	mu            sync.Mutex
	destinations  map[uuid.UUID]models.EventDestination
	subscriptions map[uuid.UUID]models.EventSubscription
	deliveries    []models.EventDelivery
}

func newMemEventStore() *memEventStore {
	return &memEventStore{
		destinations:  map[uuid.UUID]models.EventDestination{},
		subscriptions: map[uuid.UUID]models.EventSubscription{},
	}
}

func ownedBy(s *models.EventSubscription, t repositories.EventTarget) bool {
	return s.OUID == t.OUID && s.Scope == t.Scope && s.ProjectName == t.ProjectName && s.AgentName == t.AgentName
}

func (m *memEventStore) usingLocked(ouID string, id uuid.UUID) []models.EventSubscription {
	var out []models.EventSubscription
	for _, s := range m.subscriptions {
		if s.OUID == ouID && s.DestinationID == id {
			out = append(out, s)
		}
	}
	return out
}

func (m *memEventStore) saveSubscription(
	s *models.EventSubscription, check func(*models.EventDestination) error,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.destinations[s.DestinationID]
	if !ok || d.OUID != s.OUID {
		return gorm.ErrRecordNotFound
	}
	if check != nil {
		if err := check(&d); err != nil {
			return err
		}
	}
	m.subscriptions[s.ID] = *s
	return nil
}

func (m *memEventStore) mock() *repomocks.EventRepositoryMock {
	return &repomocks.EventRepositoryMock{
		ListDestinationsFunc: func(_ context.Context, ouID string) ([]models.EventDestination, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			out := []models.EventDestination{}
			for _, d := range m.destinations {
				if d.OUID == ouID {
					out = append(out, d)
				}
			}
			return out, nil
		},
		GetDestinationFunc: func(_ context.Context, ouID string, id uuid.UUID) (*models.EventDestination, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			d, ok := m.destinations[id]
			if !ok || d.OUID != ouID {
				return nil, gorm.ErrRecordNotFound
			}
			return &d, nil
		},
		DestinationsByIDFunc: func(_ context.Context, ouID string, ids []uuid.UUID) ([]models.EventDestination, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			out := []models.EventDestination{}
			for _, id := range ids {
				if d, ok := m.destinations[id]; ok && d.OUID == ouID {
					out = append(out, d)
				}
			}
			return out, nil
		},
		CreateDestinationFunc: func(_ context.Context, d *models.EventDestination) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			for _, other := range m.destinations {
				if other.OUID == d.OUID && strings.EqualFold(other.Name, d.Name) {
					return utils.ErrEventDestinationNameTaken
				}
			}
			m.destinations[d.ID] = *d
			return nil
		},
		UpdateDestinationFunc: func(
			_ context.Context, d *models.EventDestination, check func([]models.EventSubscription) error,
		) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			if _, ok := m.destinations[d.ID]; !ok {
				return gorm.ErrRecordNotFound
			}
			if check != nil {
				if err := check(m.usingLocked(d.OUID, d.ID)); err != nil {
					return err
				}
			}
			m.destinations[d.ID] = *d
			return nil
		},
		DeleteDestinationFunc: func(_ context.Context, ouID string, id uuid.UUID) (bool, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			d, ok := m.destinations[id]
			if !ok || d.OUID != ouID {
				return false, nil
			}
			if len(m.usingLocked(ouID, id)) > 0 {
				return false, utils.ErrEventDestinationInUse
			}
			delete(m.destinations, id)
			return true, nil
		},
		DestinationStatsFunc: func(_ context.Context, ouID string) (map[uuid.UUID]repositories.DestinationStats, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			out := map[uuid.UUID]repositories.DestinationStats{}
			for id, d := range m.destinations {
				if d.OUID == ouID {
					out[id] = repositories.DestinationStats{UsedBy: len(m.usingLocked(ouID, id))}
				}
			}
			return out, nil
		},
		SubscriptionsUsingFunc: func(_ context.Context, ouID string, id uuid.UUID) ([]models.EventSubscription, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.usingLocked(ouID, id), nil
		},
		RemoveProjectFromDestinationsFunc: func(_ context.Context, ouID, project string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			for id, d := range m.destinations {
				if d.OUID == ouID && d.Availability == models.DestinationAvailableProjects {
					d.Projects = slices.DeleteFunc(slices.Clone(d.Projects), func(p string) bool { return p == project })
					m.destinations[id] = d
				}
			}
			return nil
		},
		ListSubscriptionsFunc: func(_ context.Context, t repositories.EventTarget) ([]models.EventSubscription, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			out := []models.EventSubscription{}
			for _, s := range m.subscriptions {
				if ownedBy(&s, t) {
					out = append(out, s)
				}
			}
			slices.SortFunc(out, func(a, b models.EventSubscription) int { return strings.Compare(a.Name, b.Name) })
			return out, nil
		},
		GetSubscriptionFunc: func(_ context.Context, t repositories.EventTarget, id uuid.UUID) (*models.EventSubscription, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			s, ok := m.subscriptions[id]
			if !ok || !ownedBy(&s, t) {
				return nil, gorm.ErrRecordNotFound
			}
			return &s, nil
		},
		SubscriptionsByIDFunc: func(_ context.Context, ouID string, ids []uuid.UUID) ([]models.EventSubscription, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			out := []models.EventSubscription{}
			for _, id := range ids {
				if s, ok := m.subscriptions[id]; ok && s.OUID == ouID {
					out = append(out, s)
				}
			}
			return out, nil
		},
		CreateSubscriptionFunc: func(
			_ context.Context, s *models.EventSubscription, check func(*models.EventDestination) error,
		) error {
			return m.saveSubscription(s, check)
		},
		UpdateSubscriptionFunc: func(
			_ context.Context, s *models.EventSubscription, check func(*models.EventDestination) error,
		) error {
			return m.saveSubscription(s, check)
		},
		DeleteSubscriptionFunc: func(_ context.Context, t repositories.EventTarget, id uuid.UUID) (bool, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			s, ok := m.subscriptions[id]
			if !ok || !ownedBy(&s, t) {
				return false, nil
			}
			delete(m.subscriptions, id)
			return true, nil
		},
		DeleteSubscriptionsUnderFunc: func(_ context.Context, ouID, project, agent string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			for id, s := range m.subscriptions {
				if s.OUID == ouID && s.ProjectName == project && (agent == "" || s.AgentName == agent) {
					delete(m.subscriptions, id)
				}
			}
			return nil
		},
		SubscriptionCountsFunc: func(
			context.Context, string, []uuid.UUID, time.Time, repositories.DeliveryLocation,
		) (repositories.SubscriptionCounts, error) {
			return repositories.SubscriptionCounts{}, nil
		},
		ListDeliveriesFunc: func(_ context.Context, f repositories.DeliveryFilter) ([]models.EventDelivery, int64, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			var out []models.EventDelivery
			for _, d := range m.deliveries {
				if d.OUID == f.OUID {
					out = append(out, d)
				}
			}
			return out, int64(len(out)), nil
		},
	}
}

func newTestDestinationService(drivers ...eventdestinations.Driver) (EventDestinationService, *memEventStore, *repomocks.EventRepositoryMock) {
	store := newMemEventStore()
	repo := store.mock()
	svc := NewEventDestinationService(repo, eventdestinations.NewRegistry(drivers...), testEventKey, slog.New(slog.DiscardHandler))
	return svc, store, repo
}

func validDestinationInput() EventDestinationInput {
	return EventDestinationInput{
		Name:         "  Ops channel ",
		Kind:         "fake",
		Config:       json.RawMessage(`{"dest":"somewhere","ignored":true}`),
		Availability: models.DestinationAvailableProjects,
		Projects:     []string{"payments", " payments", " ", "claims"},
	}
}

// putSubscription stores a subscription of target sending to destination.
func putSubscription(store *memEventStore, t repositories.EventTarget, name string, destination uuid.UUID) models.EventSubscription {
	s := models.EventSubscription{
		ID: uuid.New(), OUID: t.OUID, Scope: t.Scope, ProjectName: t.ProjectName, AgentName: t.AgentName,
		Name: name, DestinationID: destination, EventTypes: []string{events.AllEvents}, Enabled: true,
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.subscriptions[s.ID] = s
	return s
}

func TestEventDestinationService_CreateNormalisesAndAudits(t *testing.T) {
	driver := &fakeDestinationDriver{hasSecret: true}
	svc, _, _ := newTestDestinationService(driver)
	rec := &recordingAuditor{}

	d, secret, err := svc.Create(auditCtx(rec), "ou-1", validDestinationInput())
	require.NoError(t, err)
	assert.Equal(t, "Ops channel", d.Name)
	assert.Equal(t, "fake", d.Kind)
	assert.JSONEq(t, `{"dest":"somewhere"}`, string(d.Config), "the driver normalises the config")
	assert.Equal(t, []string{"payments", "claims"}, d.Projects, "projects are trimmed and deduplicated")
	assert.NotEmpty(t, secret)
	assert.NotContains(t, string(d.SecretEncrypted), secret, "the secret is stored encrypted")
	assert.Equal(t, []audit.Action{audit.ActionEventDestinationCreate, audit.ActionEventDestinationCreate}, rec.actions(),
		"intent and outcome are recorded")
}

func TestEventDestinationService_Validation(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{})
	ctx := auditCtx(&recordingAuditor{})
	cases := map[string]func(*EventDestinationInput){
		"missing name":             func(in *EventDestinationInput) { in.Name = " " },
		"long name":                func(in *EventDestinationInput) { in.Name = strings.Repeat("n", 101) },
		"unknown kind":             func(in *EventDestinationInput) { in.Kind = "carrier-pigeon" },
		"invalid config":           func(in *EventDestinationInput) { in.Config = json.RawMessage(`{}`) },
		"unknown availability":     func(in *EventDestinationInput) { in.Availability = "everyone" },
		"projects without project": func(in *EventDestinationInput) { in.Projects = []string{" "} },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			in := validDestinationInput()
			edit(&in)
			_, _, err := svc.Create(ctx, "ou-1", in)
			assert.ErrorIs(t, err, utils.ErrInvalidInput)
		})
	}

	in := validDestinationInput()
	in.Availability = models.DestinationAvailableAll
	d, _, err := svc.Create(ctx, "ou-1", in)
	require.NoError(t, err)
	assert.Empty(t, d.Projects, "only a projects destination lists projects")
}

func TestEventDestinationService_NamesAreUniquePerOrg(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{})
	ctx := auditCtx(&recordingAuditor{})
	_, _, err := svc.Create(ctx, "ou-1", validDestinationInput())
	require.NoError(t, err)

	in := validDestinationInput()
	in.Name = "OPS CHANNEL"
	_, _, err = svc.Create(ctx, "ou-1", in)
	assert.ErrorIs(t, err, utils.ErrEventDestinationNameTaken)

	_, _, err = svc.Create(ctx, "ou-2", in)
	assert.NoError(t, err, "another org may use the name")
}

func TestEventDestinationService_KindWithoutSecret(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{})
	ctx := auditCtx(&recordingAuditor{})
	d, secret, err := svc.Create(ctx, "ou-1", validDestinationInput())
	require.NoError(t, err)
	assert.Empty(t, secret)
	assert.Empty(t, d.SecretEncrypted)

	_, err = svc.RotateSecret(ctx, "ou-1", d.ID)
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "nothing to rotate")
}

func TestEventDestinationService_RotateSecret(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{hasSecret: true})
	ctx := auditCtx(&recordingAuditor{})
	d, first, err := svc.Create(ctx, "ou-1", validDestinationInput())
	require.NoError(t, err)

	second, err := svc.RotateSecret(ctx, "ou-1", d.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	stored, err := svc.Get(ctx, "ou-1", d.ID)
	require.NoError(t, err)
	_, dest, err := svc.Resolve(stored)
	require.NoError(t, err)
	assert.Equal(t, second, dest.Secret, "the new secret is the one used to sign")
}

func TestEventDestinationService_UpdateCannotChangeKind(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{}, &credentialDriver{})
	ctx := auditCtx(&recordingAuditor{})
	d, _, err := svc.Create(ctx, "ou-1", validDestinationInput())
	require.NoError(t, err)

	in := validDestinationInput()
	in.Kind = "cred"
	_, err = svc.Update(ctx, "ou-1", d.ID, in)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestEventDestinationService_OrgIsolation(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{hasSecret: true})
	ctx := auditCtx(&recordingAuditor{})
	d, _, err := svc.Create(ctx, "ou-1", validDestinationInput())
	require.NoError(t, err)

	_, err = svc.Get(ctx, "ou-2", d.ID)
	assert.ErrorIs(t, err, utils.ErrEventDestinationNotFound)
	_, err = svc.Update(ctx, "ou-2", d.ID, validDestinationInput())
	assert.ErrorIs(t, err, utils.ErrEventDestinationNotFound)
	assert.ErrorIs(t, svc.Delete(ctx, "ou-2", d.ID), utils.ErrEventDestinationNotFound)
	_, err = svc.RotateSecret(ctx, "ou-2", d.ID)
	assert.ErrorIs(t, err, utils.ErrEventDestinationNotFound)
	_, err = svc.Test(ctx, "ou-2", d.ID)
	assert.ErrorIs(t, err, utils.ErrEventDestinationNotFound)
}

func TestEventDestinationService_DeleteInUse(t *testing.T) {
	svc, store, repo := newTestDestinationService(&fakeDestinationDriver{})
	ctx := auditCtx(&recordingAuditor{})
	d, _, err := svc.Create(ctx, "ou-1", validDestinationInput())
	require.NoError(t, err)
	sub := putSubscription(store, repositories.EventTarget{OUID: "ou-1", Scope: models.EventScopeProject, ProjectName: "payments"},
		"Deploys", d.ID)

	err = svc.Delete(ctx, "ou-1", d.ID)
	require.ErrorIs(t, err, utils.ErrEventDestinationInUse)
	assert.Contains(t, err.Error(), `"Deploys" (project payments)`, "the error names who still uses it")
	assert.Empty(t, repo.DeleteDestinationCalls(), "nothing is deleted while in use")

	// A subscription added after the check is caught by the foreign key.
	store.mu.Lock()
	delete(store.subscriptions, sub.ID)
	store.mu.Unlock()
	repo.SubscriptionsUsingFunc = func(context.Context, string, uuid.UUID) ([]models.EventSubscription, error) {
		return []models.EventSubscription{}, nil
	}
	putSubscription(store, repositories.EventTarget{OUID: "ou-1", Scope: models.EventScopeOrg}, "Late", d.ID)
	assert.ErrorIs(t, svc.Delete(ctx, "ou-1", d.ID), utils.ErrEventDestinationInUse)
}

func TestEventDestinationService_NarrowingAvailabilityInUse(t *testing.T) {
	svc, store, _ := newTestDestinationService(&fakeDestinationDriver{})
	ctx := auditCtx(&recordingAuditor{})
	d, _, err := svc.Create(ctx, "ou-1", validDestinationInput()) // payments, claims
	require.NoError(t, err)
	putSubscription(store, repositories.EventTarget{
		OUID: "ou-1", Scope: models.EventScopeAgent, ProjectName: "claims", AgentName: "triage",
	}, "Claims deploys", d.ID)
	putSubscription(store, repositories.EventTarget{OUID: "ou-1", Scope: models.EventScopeOrg}, "Audit", d.ID)

	in := validDestinationInput()
	in.Projects = []string{"payments"}
	_, err = svc.Update(ctx, "ou-1", d.ID, in)
	require.ErrorIs(t, err, utils.ErrEventDestinationInUse, "the claims agent would lose its destination")
	assert.Contains(t, err.Error(), `"Claims deploys" (agent claims/triage)`)

	in.Availability = models.DestinationAvailableOrg
	_, err = svc.Update(ctx, "ou-1", d.ID, in)
	assert.ErrorIs(t, err, utils.ErrEventDestinationInUse)

	in.Availability = models.DestinationAvailableAll
	updated, err := svc.Update(ctx, "ou-1", d.ID, in)
	require.NoError(t, err, "widening is always allowed; the org subscription never loses access")
	assert.Equal(t, models.DestinationAvailableAll, updated.Availability)
}

func TestEventDestinationService_AuditFailureBlocksTheChange(t *testing.T) {
	svc, _, repo := newTestDestinationService(&fakeDestinationDriver{})
	rec := &recordingAuditor{fail: errors.New("sink down")}
	_, _, err := svc.Create(auditCtx(rec), "ou-1", validDestinationInput())
	require.Error(t, err)
	assert.Empty(t, repo.CreateDestinationCalls(), "nothing is stored without an audit record")
}

func TestEventDestinationService_TestSendsTestEvent(t *testing.T) {
	driver := &fakeDestinationDriver{
		hasSecret: true, result: eventdestinations.Result{StatusCode: 503, Err: errors.New("down")},
	}
	svc, _, _ := newTestDestinationService(driver)
	ctx := auditCtx(&recordingAuditor{})
	d, secret, err := svc.Create(ctx, "ou-1", validDestinationInput())
	require.NoError(t, err)

	res, err := svc.Test(ctx, "ou-1", d.ID)
	require.NoError(t, err)
	assert.False(t, res.Delivered)
	assert.Equal(t, 503, res.StatusCode)
	assert.Equal(t, "down", res.Error)

	require.Len(t, driver.msgs, 1)
	assert.Equal(t, secret, driver.sent[0].Secret, "the decrypted secret reaches the driver")
	assert.Equal(t, events.TypePrefix+events.TypeDestinationTest, driver.msgs[0].Type)
	var evt events.Event
	require.NoError(t, json.Unmarshal(driver.msgs[0].Body, &evt))
	assert.Equal(t, driver.msgs[0].ID, evt.ID)
	assert.Equal(t, "ou-1", evt.OrgID)
}

func TestEventDestinationService_Available(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{})
	ctx := auditCtx(&recordingAuditor{})
	create := func(name, availability string, projects ...string) {
		in := validDestinationInput()
		in.Name, in.Availability, in.Projects = name, availability, projects
		_, _, err := svc.Create(ctx, "ou-1", in)
		require.NoError(t, err)
	}
	create("Everyone", models.DestinationAvailableAll)
	create("Admins only", models.DestinationAvailableOrg)
	create("Payments", models.DestinationAvailableProjects, "payments")

	names := func(project string) []string {
		list, _, err := svc.Available(ctx, "ou-1", project)
		require.NoError(t, err)
		var out []string
		for _, d := range list {
			out = append(out, d.Name)
		}
		slices.Sort(out)
		return out
	}
	assert.Equal(t, []string{"Everyone", "Payments"}, names("payments"))
	assert.Equal(t, []string{"Everyone"}, names("claims"))
}

func TestEventDestinationService_SuppliedCredential(t *testing.T) {
	svc, _, _ := newTestDestinationService(&credentialDriver{})
	ctx := auditCtx(&recordingAuditor{})
	in := validDestinationInput()
	in.Kind = "cred"

	in.Config = json.RawMessage(`{"dest":"a"}`)
	_, _, err := svc.Create(ctx, "ou-1", in)
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "a new destination needs its credential")

	in.Config = json.RawMessage(`{"dest":"a","token":"tok-1"}`)
	d, secret, err := svc.Create(ctx, "ou-1", in)
	require.NoError(t, err)
	assert.Empty(t, secret, "a supplied credential is never returned")
	assert.JSONEq(t, `{"dest":"a"}`, string(d.Config), "the credential is not stored in plain")
	_, dest, err := svc.Resolve(d)
	require.NoError(t, err)
	assert.Equal(t, "tok-1", dest.Secret, "it is stored encrypted and resolved for sending")

	// Leaving the credential out keeps it.
	in.Config = json.RawMessage(`{"dest":"a"}`)
	in.Description = "renamed"
	d, err = svc.Update(ctx, "ou-1", d.ID, in)
	require.NoError(t, err)
	_, dest, _ = svc.Resolve(d)
	assert.Equal(t, "tok-1", dest.Secret)

	in.Config = json.RawMessage(`{"dest":"b"}`)
	_, err = svc.Update(ctx, "ou-1", d.ID, in)
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "other settings need the credential again")

	in.Config = json.RawMessage(`{"dest":"b","token":"tok-2"}`)
	d, err = svc.Update(ctx, "ou-1", d.ID, in)
	require.NoError(t, err)
	_, dest, _ = svc.Resolve(d)
	assert.Equal(t, "tok-2", dest.Secret, "a new credential replaces the old one")
}

func TestEventDestinationService_RedactedConfigAndService(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{})
	assert.JSONEq(t, `{"dest":"masked"}`, string(svc.RedactedConfig(&models.EventDestination{Kind: "fake"})))
	assert.JSONEq(t, `{}`, string(svc.RedactedConfig(&models.EventDestination{Kind: "unknown"})))
	assert.Equal(t, "slack", svc.Service(&models.EventDestination{
		Kind: models.DestinationKindNotify, Config: json.RawMessage(`{"service":"slack"}`),
	}))
	assert.Empty(t, svc.Service(&models.EventDestination{Kind: models.DestinationKindWebhook}))
}

func TestDestinationHealthOf(t *testing.T) {
	assert.Equal(t, DestinationIdle, DestinationHealthOf(repositories.DestinationStats{}))
	assert.Equal(t, DestinationHealthy, DestinationHealthOf(repositories.DestinationStats{
		LastStatus: models.EventDeliveryDelivered,
	}))
	assert.Equal(t, DestinationHealthy, DestinationHealthOf(repositories.DestinationStats{
		LastStatus: models.EventDeliveryPending,
	}), "a queued delivery after a success is not a failure")
	assert.Equal(t, DestinationFailing, DestinationHealthOf(repositories.DestinationStats{
		LastStatus: models.EventDeliveryPending, ConsecutiveFailures: 3,
	}))
}
