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
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	maxEventDestinationsPerOrg = 50
	maxEventNameLength         = 100
	maxEventDescriptionLength  = 500
	destinationTestMessage     = "Test event from WSO2 Agent Manager. No action is needed."
	// maxNamedSubscriptions bounds how many subscriptions an error names.
	maxNamedSubscriptions = 5
)

// Destination health, derived from its recent deliveries.
const (
	// DestinationHealthy: the latest delivery succeeded.
	DestinationHealthy = "healthy"
	// DestinationFailing: deliveries failed, or are being retried, since the
	// latest one that succeeded.
	DestinationFailing = "failing"
	// DestinationIdle: nothing has been delivered to it yet.
	DestinationIdle = "idle"
)

// DestinationHealthOf derives a destination's health from its stats.
func DestinationHealthOf(s repositories.DestinationStats) string {
	switch {
	case s.ConsecutiveFailures > 0:
		return DestinationFailing
	case s.LastStatus == "":
		return DestinationIdle
	default:
		return DestinationHealthy
	}
}

// EventDestinationInput is the input for creating or updating a destination.
type EventDestinationInput struct {
	Name        string
	Description string
	// Kind decides how events are delivered; it cannot change on update.
	Kind string
	// Config is the kind's settings, validated by its driver.
	Config json.RawMessage
	// Availability says which subscriptions below the org may use the
	// destination: all, org (none) or projects (those of Projects).
	Availability string
	Projects     []string
}

// DestinationKind describes a supported kind of destination.
type DestinationKind struct {
	Kind string
	// GeneratedSecret is set for kinds whose destinations get a secret that
	// can be rotated.
	GeneratedSecret bool
	// SuppliedCredential is set for kinds whose settings include a
	// credential the user enters, which is never returned.
	SuppliedCredential bool
	// Services lists the services the kind delivers to, if it has several.
	Services []eventdestinations.Service
}

// DestinationTestResult is the outcome of a test send.
type DestinationTestResult struct {
	Delivered  bool
	StatusCode int
	Error      string
}

// DestinationResolver turns a stored destination into its driver and a
// destination with the secret decrypted, ready to deliver to.
type DestinationResolver interface {
	Resolve(d *models.EventDestination) (eventdestinations.Driver, eventdestinations.Destination, error)
}

// EventDestinationService manages an org's event destinations. Every change
// is recorded fail-closed in the audit trail before it applies.
type EventDestinationService interface {
	// Kinds lists the supported destination kinds.
	Kinds() []DestinationKind
	// List returns the org's destinations and their stats.
	List(ctx context.Context, ouID string) ([]models.EventDestination, map[uuid.UUID]repositories.DestinationStats, error)
	// Available returns the destinations a subscription of project may send
	// to, and their stats.
	Available(ctx context.Context, ouID, project string) ([]models.EventDestination, map[uuid.UUID]repositories.DestinationStats, error)
	Get(ctx context.Context, ouID string, id uuid.UUID) (*models.EventDestination, error)
	// Usage returns the subscriptions that send to a destination.
	Usage(ctx context.Context, ouID string, id uuid.UUID) ([]models.EventSubscription, error)
	// Create returns the new destination and its generated secret, if its
	// kind has one. The secret is shown once and never again.
	Create(ctx context.Context, ouID string, in EventDestinationInput) (*models.EventDestination, string, error)
	// Update changes a destination. Making it unavailable to subscriptions
	// that send to it fails with utils.ErrEventDestinationInUse.
	Update(ctx context.Context, ouID string, id uuid.UUID, in EventDestinationInput) (*models.EventDestination, error)
	// Delete removes a destination no subscription sends to; otherwise it
	// fails with utils.ErrEventDestinationInUse.
	Delete(ctx context.Context, ouID string, id uuid.UUID) error
	// RotateSecret replaces the destination's generated secret and returns it.
	RotateSecret(ctx context.Context, ouID string, id uuid.UUID) (string, error)
	// Test sends a destination.test event straight to the destination.
	Test(ctx context.Context, ouID string, id uuid.UUID) (*DestinationTestResult, error)
	// RedactedConfig masks the parts of a destination's settings that may
	// carry a credential, for callers who may see it but not change it.
	RedactedConfig(d *models.EventDestination) json.RawMessage
	// Service names the service a destination delivers to: the notification
	// service, or empty for a webhook.
	Service(d *models.EventDestination) string
	DestinationResolver
}

type eventDestinationService struct {
	repo          repositories.EventRepository
	drivers       *eventdestinations.Registry
	encryptionKey []byte
	logger        *slog.Logger
	now           func() time.Time
}

// NewEventDestinationService creates an EventDestinationService.
func NewEventDestinationService(
	repo repositories.EventRepository, drivers *eventdestinations.Registry, encryptionKey []byte, logger *slog.Logger,
) EventDestinationService {
	return &eventDestinationService{repo: repo, drivers: drivers, encryptionKey: encryptionKey, logger: logger, now: time.Now}
}

func (s *eventDestinationService) Kinds() []DestinationKind {
	drivers := s.drivers.All()
	out := make([]DestinationKind, 0, len(drivers))
	for _, d := range drivers {
		k := DestinationKind{Kind: d.Kind(), Services: []eventdestinations.Service{}}
		// Asking for a secret only to learn whether the kind has one is
		// cheap: it is a few random bytes, thrown away.
		_, k.GeneratedSecret, _ = d.NewSecret()
		_, k.SuppliedCredential = d.(eventdestinations.CredentialDriver)
		if c, ok := d.(eventdestinations.ServiceCatalog); ok {
			k.Services = c.Services()
		}
		out = append(out, k)
	}
	return out
}

func (s *eventDestinationService) List(
	ctx context.Context, ouID string,
) ([]models.EventDestination, map[uuid.UUID]repositories.DestinationStats, error) {
	list, err := s.repo.ListDestinations(ctx, ouID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list event destinations: %w", err)
	}
	stats, err := s.repo.DestinationStats(ctx, ouID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read event destination stats: %w", err)
	}
	return list, stats, nil
}

func (s *eventDestinationService) Available(
	ctx context.Context, ouID, project string,
) ([]models.EventDestination, map[uuid.UUID]repositories.DestinationStats, error) {
	list, stats, err := s.List(ctx, ouID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]models.EventDestination, 0, len(list))
	for i := range list {
		if list[i].UsableBy(models.EventScopeProject, project) {
			out = append(out, list[i])
		}
	}
	return out, stats, nil
}

func (s *eventDestinationService) Get(ctx context.Context, ouID string, id uuid.UUID) (*models.EventDestination, error) {
	d, err := s.repo.GetDestination(ctx, ouID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrEventDestinationNotFound
		}
		return nil, fmt.Errorf("failed to get event destination: %w", err)
	}
	return d, nil
}

func (s *eventDestinationService) Usage(ctx context.Context, ouID string, id uuid.UUID) ([]models.EventSubscription, error) {
	if _, err := s.Get(ctx, ouID, id); err != nil {
		return nil, err
	}
	using, err := s.repo.SubscriptionsUsing(ctx, ouID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list the subscriptions of an event destination: %w", err)
	}
	return using, nil
}

func (s *eventDestinationService) Create(
	ctx context.Context, ouID string, in EventDestinationInput,
) (*models.EventDestination, string, error) {
	driver, credential, err := s.validate(ctx, &in)
	if err != nil {
		return nil, "", err
	}
	if _, ok := driver.(eventdestinations.CredentialDriver); ok && credential == "" {
		return nil, "", fmt.Errorf("%w: url is required", utils.ErrInvalidInput)
	}
	existing, err := s.repo.ListDestinations(ctx, ouID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list event destinations: %w", err)
	}
	if len(existing) >= maxEventDestinationsPerOrg {
		return nil, "", fmt.Errorf("%w: an organization can have at most %d event destinations",
			utils.ErrInvalidInput, maxEventDestinationsPerOrg)
	}
	secret, encrypted, err := s.newSecret(driver)
	if err != nil {
		return nil, "", err
	}
	if credential != "" {
		if encrypted, err = s.encrypt(credential); err != nil {
			return nil, "", err
		}
	}
	d := &models.EventDestination{
		ID:              uuid.New(),
		OUID:            ouID,
		Name:            in.Name,
		Description:     in.Description,
		Kind:            in.Kind,
		Config:          in.Config,
		SecretEncrypted: encrypted,
		Availability:    in.Availability,
		Projects:        in.Projects,
		CreatedBy:       audit.BuildEvent(ctx, audit.ActionEventDestinationCreate).ActorID,
	}
	attempt, err := audit.Begin(ctx, audit.ActionEventDestinationCreate, s.auditOpts(ouID, d, driver)...)
	if err != nil {
		return nil, "", err
	}
	if err := s.repo.CreateDestination(ctx, d); err != nil {
		if !errors.Is(err, utils.ErrEventDestinationNameTaken) {
			err = fmt.Errorf("failed to create event destination: %w", err)
		}
		attempt.Complete(ctx, err)
		return nil, "", err
	}
	attempt.Complete(ctx, nil)
	return d, secret, nil
}

func (s *eventDestinationService) Update(
	ctx context.Context, ouID string, id uuid.UUID, in EventDestinationInput,
) (*models.EventDestination, error) {
	d, err := s.Get(ctx, ouID, id)
	if err != nil {
		return nil, err
	}
	if in.Kind != d.Kind {
		return nil, fmt.Errorf("%w: the kind of an event destination cannot be changed", utils.ErrInvalidInput)
	}
	driver, credential, err := s.validate(ctx, &in)
	if err != nil {
		return nil, err
	}
	if credential != "" {
		if d.SecretEncrypted, err = s.encrypt(credential); err != nil {
			return nil, err
		}
	} else if _, ok := driver.(eventdestinations.CredentialDriver); ok && !sameJSON(in.Config, d.Config) {
		// The stored credential belongs to the old settings (another
		// service's URL), so it cannot be kept.
		return nil, fmt.Errorf("%w: enter the url again when changing the service", utils.ErrInvalidInput)
	}
	d.Name, d.Description, d.Config = in.Name, in.Description, in.Config
	d.Availability, d.Projects = in.Availability, in.Projects
	attempt, err := audit.Begin(ctx, audit.ActionEventDestinationUpdate, s.auditOpts(ouID, d, driver)...)
	if err != nil {
		return nil, err
	}
	err = s.repo.UpdateDestination(ctx, d, func(using []models.EventSubscription) error {
		var stranded []models.EventSubscription
		for i := range using {
			if !d.UsableBy(using[i].Scope, using[i].ProjectName) {
				stranded = append(stranded, using[i])
			}
		}
		if len(stranded) > 0 {
			return fmt.Errorf("%w: it would no longer be available to %s",
				utils.ErrEventDestinationInUse, describeSubscriptions(stranded))
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = utils.ErrEventDestinationNotFound
		case errors.Is(err, utils.ErrEventDestinationInUse), errors.Is(err, utils.ErrEventDestinationNameTaken):
		default:
			err = fmt.Errorf("failed to update event destination: %w", err)
		}
		attempt.Complete(ctx, err)
		return nil, err
	}
	attempt.Complete(ctx, nil)
	return d, nil
}

func (s *eventDestinationService) Delete(ctx context.Context, ouID string, id uuid.UUID) error {
	d, err := s.Get(ctx, ouID, id)
	if err != nil {
		return err
	}
	using, err := s.repo.SubscriptionsUsing(ctx, ouID, id)
	if err != nil {
		return fmt.Errorf("failed to list the subscriptions of an event destination: %w", err)
	}
	if len(using) > 0 {
		return fmt.Errorf("%w: delete or move %s first", utils.ErrEventDestinationInUse, describeSubscriptions(using))
	}
	driver, _ := s.drivers.Get(d.Kind)
	attempt, err := audit.Begin(ctx, audit.ActionEventDestinationDelete, s.auditOpts(ouID, d, driver)...)
	if err != nil {
		return err
	}
	// The foreign key catches a subscription added since the check above.
	deleted, err := s.repo.DeleteDestination(ctx, ouID, id)
	switch {
	case errors.Is(err, utils.ErrEventDestinationInUse):
	case err != nil:
		err = fmt.Errorf("failed to delete event destination: %w", err)
	case !deleted:
		err = utils.ErrEventDestinationNotFound
	}
	attempt.Complete(ctx, err)
	return err
}

func (s *eventDestinationService) RotateSecret(ctx context.Context, ouID string, id uuid.UUID) (string, error) {
	d, err := s.Get(ctx, ouID, id)
	if err != nil {
		return "", err
	}
	driver, ok := s.drivers.Get(d.Kind)
	if !ok {
		return "", fmt.Errorf("%w: event destination kind %q is not supported", utils.ErrInvalidInput, d.Kind)
	}
	secret, encrypted, err := s.newSecret(driver)
	if err != nil {
		return "", err
	}
	if secret == "" {
		return "", fmt.Errorf("%w: %s destinations have no generated secret", utils.ErrInvalidInput, d.Kind)
	}
	attempt, err := audit.Begin(ctx, audit.ActionEventDestinationRotateSecret, s.auditOpts(ouID, d, driver)...)
	if err != nil {
		return "", err
	}
	d.SecretEncrypted = encrypted
	if err := s.repo.UpdateDestination(ctx, d, nil); err != nil {
		err = fmt.Errorf("failed to rotate event destination secret: %w", err)
		attempt.Complete(ctx, err)
		return "", err
	}
	attempt.Complete(ctx, nil)
	return secret, nil
}

func (s *eventDestinationService) Test(ctx context.Context, ouID string, id uuid.UUID) (*DestinationTestResult, error) {
	d, err := s.Get(ctx, ouID, id)
	if err != nil {
		return nil, err
	}
	driver, dest, err := s.Resolve(d)
	if err != nil {
		return nil, err
	}
	evt := events.New("evt_"+uuid.NewString(), events.TypeDestinationTest, events.ScopeOrg, d.OUID, "", "", s.now())
	evt.Subject = d.ID.String()
	evt.Data = map[string]any{"message": destinationTestMessage, "destinationId": d.ID.String(), "name": d.Name}
	body, err := json.Marshal(evt)
	if err != nil {
		return nil, fmt.Errorf("failed to encode test event: %w", err)
	}
	res := driver.Deliver(ctx, dest, eventdestinations.Message{ID: evt.ID, Type: evt.Type, Body: body})
	out := &DestinationTestResult{Delivered: res.Err == nil, StatusCode: res.StatusCode}
	if res.Err != nil {
		out.Error = res.Err.Error()
	}
	return out, nil
}

func (s *eventDestinationService) RedactedConfig(d *models.EventDestination) json.RawMessage {
	driver, ok := s.drivers.Get(d.Kind)
	if !ok {
		return json.RawMessage(`{}`)
	}
	return driver.RedactConfig(d.Config)
}

func (s *eventDestinationService) Service(d *models.EventDestination) string {
	if d.Kind != models.DestinationKindNotify {
		return ""
	}
	var cfg struct {
		Service string `json:"service"`
	}
	_ = json.Unmarshal(d.Config, &cfg)
	return cfg.Service
}

// Resolve returns a destination's driver and the destination to deliver to,
// with the secret decrypted.
func (s *eventDestinationService) Resolve(
	d *models.EventDestination,
) (eventdestinations.Driver, eventdestinations.Destination, error) {
	driver, ok := s.drivers.Get(d.Kind)
	if !ok {
		return nil, eventdestinations.Destination{},
			fmt.Errorf("%w: event destination kind %q is not supported", utils.ErrInvalidInput, d.Kind)
	}
	dest := eventdestinations.Destination{Config: d.Config}
	if len(d.SecretEncrypted) > 0 {
		secret, err := utils.DecryptBytes(d.SecretEncrypted, s.encryptionKey)
		if err != nil {
			return nil, eventdestinations.Destination{}, fmt.Errorf("failed to decrypt event destination secret: %w", err)
		}
		dest.Secret = string(secret)
	}
	return driver, dest, nil
}

// validate checks and normalises an input and returns the driver of its
// kind. For a kind whose settings carry a credential, the credential is taken
// out of in.Config and returned; it is empty when the input leaves it out.
func (s *eventDestinationService) validate(
	ctx context.Context, in *EventDestinationInput,
) (eventdestinations.Driver, string, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || len(in.Name) > maxEventNameLength {
		return nil, "", fmt.Errorf("%w: name is required and must be at most %d characters", utils.ErrInvalidInput, maxEventNameLength)
	}
	if len(in.Description) > maxEventDescriptionLength {
		return nil, "", fmt.Errorf("%w: description must be at most %d characters", utils.ErrInvalidInput, maxEventDescriptionLength)
	}
	switch in.Availability {
	case models.DestinationAvailableAll, models.DestinationAvailableOrg:
		in.Projects = []string{}
	case models.DestinationAvailableProjects:
		in.Projects = uniqueTrimmed(in.Projects)
		if len(in.Projects) == 0 {
			return nil, "", fmt.Errorf("%w: select at least one project to make the destination available to", utils.ErrInvalidInput)
		}
	default:
		return nil, "", fmt.Errorf("%w: availability must be all, org or projects", utils.ErrInvalidInput)
	}
	driver, ok := s.drivers.Get(in.Kind)
	if !ok {
		return nil, "", fmt.Errorf("%w: kind must be one of %s", utils.ErrInvalidInput, strings.Join(s.drivers.Kinds(), ", "))
	}
	cfg, err := driver.NormalizeConfig(ctx, in.Config)
	if err != nil {
		return nil, "", err
	}
	var credential string
	if cd, ok := driver.(eventdestinations.CredentialDriver); ok {
		cfg, credential = cd.SplitCredential(cfg)
	}
	in.Config = cfg
	return driver, credential, nil
}

// newSecret generates and encrypts a secret for a destination of driver's
// kind. It returns empty values for kinds without a secret.
func (s *eventDestinationService) newSecret(driver eventdestinations.Driver) (string, []byte, error) {
	secret, ok, err := driver.NewSecret()
	if err != nil || !ok {
		return "", nil, err
	}
	encrypted, err := s.encrypt(secret)
	if err != nil {
		return "", nil, err
	}
	return secret, encrypted, nil
}

func (s *eventDestinationService) encrypt(secret string) ([]byte, error) {
	encrypted, err := utils.EncryptBytes([]byte(secret), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt event destination secret: %w", err)
	}
	return encrypted, nil
}

func (s *eventDestinationService) auditOpts(
	ouID string, d *models.EventDestination, driver eventdestinations.Driver,
) []audit.Option {
	opts := []audit.Option{
		audit.Org(ouID),
		audit.Detail("kind", d.Kind),
		audit.Detail("availability", d.Availability),
	}
	if driver != nil {
		// Scheme and host only: a destination path is often a credential.
		opts = append(opts, audit.Detail("destination", driver.Origin(d.Config)))
	}
	if len(d.Projects) > 0 {
		opts = append(opts, audit.Detail("projects", d.Projects))
	}
	if d.ID != uuid.Nil {
		opts = append(opts, audit.ResourceNamed(audit.ResourceEventDestination, d.ID.String(), d.Name))
	}
	return opts
}

// sameJSON reports whether a and b encode the same value; stored JSON comes
// back from the database reformatted.
func sameJSON(a, b json.RawMessage) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

// uniqueTrimmed trims values and drops empty and repeated ones, keeping the
// first occurrence's order.
func uniqueTrimmed(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// describeSubscriptions names subscriptions for an error message, with where
// each one sits: "Deploys (agent payments/refund-agent), …".
func describeSubscriptions(subs []models.EventSubscription) string {
	names := make([]string, 0, min(len(subs), maxNamedSubscriptions))
	for i := range subs {
		if i == maxNamedSubscriptions {
			break
		}
		s := &subs[i]
		var where string
		switch s.Scope {
		case models.EventScopeProject:
			where = "project " + s.ProjectName
		case models.EventScopeAgent:
			where = "agent " + s.ProjectName + "/" + s.AgentName
		default:
			where = "organization"
		}
		names = append(names, fmt.Sprintf("%q (%s)", s.Name, where))
	}
	out := strings.Join(names, ", ")
	if extra := len(subs) - len(names); extra > 0 {
		out += fmt.Sprintf(" and %d more", extra)
	}
	if len(subs) == 1 {
		return "the subscription " + out
	}
	return "the subscriptions " + out
}
