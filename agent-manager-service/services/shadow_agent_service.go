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
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Shadow Agent Discovery limits.
const (
	maxShadowConnectionsPerOrg  = 50
	maxShadowNameLength         = 100
	shadowConnectionTestTimeout = 2 * time.Minute
)

// ShadowConnectors are the connectors by platform.
type ShadowConnectors map[string]sa.Connector

// ShadowConnectionInput creates or updates a connection.
type ShadowConnectionInput struct {
	Name      string
	Provider  string
	Endpoints map[string]string
	// Settings are field values by the provider's field keys.
	Settings map[string]any
	// RemoveSecrets, on update, names optional secret fields whose stored
	// value is removed (for example the AWS session token).
	RemoveSecrets []string
}

// ShadowConnectionView is a connection as the API shows it: no secret.
type ShadowConnectionView struct {
	Connection       models.ShadowConnection
	Settings         map[string]any
	EndpointWarnings []string
}

// ShadowFindingView is an agent with its evidence and possible duplicates.
type ShadowFindingView struct {
	Row     repositories.ShadowFindingRow
	Signals []models.ShadowFindingSignal
	Related []ShadowRelationView
}

// ShadowRelationView is a relation with the related finding's name.
type ShadowRelationView struct {
	models.ShadowFindingRelation
	DisplayName string
	Provider    string
}

// ShadowAgentService is Shadow Agent Discovery: platform connections, scans
// of the platforms' agent listings, and the agents they found. Every
// credential change is recorded fail-closed in the audit trail before it
// applies.
type ShadowAgentService interface {
	ListConnections(ctx context.Context, ouID string) ([]ShadowConnectionView, error)
	GetConnection(ctx context.Context, ouID string, id uuid.UUID) (*ShadowConnectionView, error)
	CreateConnection(ctx context.Context, ouID string, in ShadowConnectionInput) (*ShadowConnectionView, error)
	UpdateConnection(ctx context.Context, ouID string, id uuid.UUID, in ShadowConnectionInput) (*ShadowConnectionView, error)
	DeleteConnection(ctx context.Context, ouID string, id uuid.UUID) error
	TestConnection(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowConnectionTestInfo, error)

	// CreateScan queues a scan of the given connections, or of every
	// connection when none is given.
	CreateScan(ctx context.Context, ouID string, connectionIDs []uuid.UUID) (*models.ShadowScan, error)
	ListScans(ctx context.Context, ouID string, limit int) ([]models.ShadowScan, error)
	GetScan(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowScan, error)
	// ClearScans deletes the org's finished scans; a queued or running scan is
	// kept. Agents are not changed.
	ClearScans(ctx context.Context, ouID string) (int, error)
	Summary(ctx context.Context, ouID string) (repositories.ShadowSummary, *models.ShadowScan, error)

	ListFindings(ctx context.Context, ouID string, f repositories.ShadowFindingFilter) ([]repositories.ShadowFindingRow, int, error)
	GetFinding(ctx context.Context, ouID string, id uuid.UUID) (*ShadowFindingView, error)
	UpdateFindingStatus(ctx context.Context, ouID string, id uuid.UUID, status string) (*ShadowFindingView, error)
	// DeleteFinding deletes an agent, on every connection that found it. A
	// later scan lists it again if a platform still has it.
	DeleteFinding(ctx context.Context, ouID string, id uuid.UUID) error
	// DeleteFindings deletes every agent of the org and returns how many.
	DeleteFindings(ctx context.Context, ouID string) (int, error)
}

type shadowAgentService struct {
	repo          repositories.ShadowAgentRepository
	connectors    ShadowConnectors
	encryptionKey []byte
	logger        *slog.Logger
	now           func() time.Time
}

// NewShadowAgentService creates a ShadowAgentService.
func NewShadowAgentService(
	repo repositories.ShadowAgentRepository, connectors ShadowConnectors, encryptionKey []byte, logger *slog.Logger,
) ShadowAgentService {
	return &shadowAgentService{
		repo: repo, connectors: connectors, encryptionKey: encryptionKey, logger: logger, now: time.Now,
	}
}

// ---- connections ----

func (s *shadowAgentService) ListConnections(ctx context.Context, ouID string) ([]ShadowConnectionView, error) {
	list, err := s.repo.ListConnections(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	out := make([]ShadowConnectionView, 0, len(list))
	for i := range list {
		out = append(out, viewOfConnection(&list[i]))
	}
	return out, nil
}

func (s *shadowAgentService) GetConnection(ctx context.Context, ouID string, id uuid.UUID) (*ShadowConnectionView, error) {
	c, err := s.loadConnection(ctx, ouID, id)
	if err != nil {
		return nil, err
	}
	v := viewOfConnection(c)
	return &v, nil
}

func (s *shadowAgentService) loadConnection(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowConnection, error) {
	c, err := s.repo.GetConnection(ctx, ouID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrShadowConnectionNotFound
		}
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}
	return c, nil
}

func (s *shadowAgentService) CreateConnection(ctx context.Context, ouID string, in ShadowConnectionInput) (*ShadowConnectionView, error) {
	spec, err := validateConnectionBasics(in)
	if err != nil {
		return nil, err
	}
	endpoints, err := resolveEndpoints(spec, nil, in.Endpoints)
	if err != nil {
		return nil, err
	}
	if len(in.RemoveSecrets) > 0 {
		return nil, fmt.Errorf("%w: removeSecrets applies only when updating a connection", utils.ErrInvalidInput)
	}
	cfg := models.ShadowConnectionConfig{Endpoints: endpoints}
	var secret models.ShadowConnectionSecret
	if err := applyShadowSettings(spec, in.Settings, &cfg, &secret, true); err != nil {
		return nil, err
	}
	if err := validateSourceHostSettings(spec, &cfg); err != nil {
		return nil, err
	}
	pruneValueEndpoints(spec, &cfg)
	if err := checkCredentialChoice(spec, secret, cfg); err != nil {
		return nil, err
	}
	existing, err := s.repo.ListConnections(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	if len(existing) >= maxShadowConnectionsPerOrg {
		return nil, fmt.Errorf("%w: an organization can have at most %d connections", utils.ErrInvalidInput, maxShadowConnectionsPerOrg)
	}
	encrypted, err := s.encryptSecret(secret)
	if err != nil {
		return nil, err
	}
	c := &models.ShadowConnection{
		ID:              uuid.New(),
		OUID:            ouID,
		Name:            strings.TrimSpace(in.Name),
		Provider:        in.Provider,
		Config:          cfg,
		SecretEncrypted: encrypted,
		SecretHints:     secretHints(secret),
		CreatedBy:       audit.BuildEvent(ctx, audit.ActionShadowConnectionCreate).ActorID,
	}
	attempt, err := audit.Begin(ctx, audit.ActionShadowConnectionCreate, connectionAuditOpts(ouID, c, true)...)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateConnection(ctx, c); err != nil {
		if !errors.Is(err, utils.ErrShadowConnectionNameTaken) {
			err = fmt.Errorf("failed to create connection: %w", err)
		}
		attempt.Complete(ctx, err)
		return nil, err
	}
	attempt.Complete(ctx, nil)
	v := viewOfConnection(c)
	return &v, nil
}

func (s *shadowAgentService) UpdateConnection(
	ctx context.Context, ouID string, id uuid.UUID, in ShadowConnectionInput,
) (*ShadowConnectionView, error) {
	c, err := s.loadConnection(ctx, ouID, id)
	if err != nil {
		return nil, err
	}
	if in.Provider == "" {
		in.Provider = c.Provider
	}
	if in.Provider != c.Provider {
		return nil, fmt.Errorf("%w: the platform of a connection cannot be changed", utils.ErrInvalidInput)
	}
	spec, err := validateConnectionBasics(in)
	if err != nil {
		return nil, err
	}
	endpoints, err := resolveEndpoints(spec, c.Config.Endpoints, in.Endpoints)
	if err != nil {
		return nil, err
	}
	secret, err := s.decryptSecret(c.SecretEncrypted)
	if err != nil {
		return nil, err
	}
	before, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("encoding the stored secret: %w", err)
	}
	if err := removeShadowSecrets(spec, in.RemoveSecrets, in.Settings, &secret); err != nil {
		return nil, err
	}
	cfg := models.ShadowConnectionConfig{Endpoints: endpoints}
	if err := applyShadowSettings(spec, in.Settings, &cfg, &secret, false); err != nil {
		return nil, err
	}
	pruneShadowSecret(spec, &secret)
	if err := validateSourceHostSettings(spec, &cfg); err != nil {
		return nil, err
	}
	pruneValueEndpoints(spec, &cfg)
	if err := checkCredentialChoice(spec, secret, cfg); err != nil {
		return nil, err
	}
	after, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("encoding the secret: %w", err)
	}
	encrypted, err := s.encryptSecret(secret)
	if err != nil {
		return nil, err
	}
	c.Name = strings.TrimSpace(in.Name)
	c.Config = cfg
	c.SecretEncrypted = encrypted
	c.SecretHints = secretHints(secret)
	attempt, err := audit.Begin(ctx, audit.ActionShadowConnectionUpdate,
		connectionAuditOpts(ouID, c, string(before) != string(after))...)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateConnection(ctx, c); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = utils.ErrShadowConnectionNotFound
		case errors.Is(err, utils.ErrShadowConnectionNameTaken):
		default:
			err = fmt.Errorf("failed to update connection: %w", err)
		}
		attempt.Complete(ctx, err)
		return nil, err
	}
	attempt.Complete(ctx, nil)
	v := viewOfConnection(c)
	return &v, nil
}

func (s *shadowAgentService) DeleteConnection(ctx context.Context, ouID string, id uuid.UUID) error {
	c, err := s.loadConnection(ctx, ouID, id)
	if err != nil {
		return err
	}
	attempt, err := audit.Begin(ctx, audit.ActionShadowConnectionDelete, connectionAuditOpts(ouID, c, true)...)
	if err != nil {
		return err
	}
	deleted, err := s.repo.DeleteConnection(ctx, ouID, id)
	switch {
	case err != nil:
		err = fmt.Errorf("failed to delete connection: %w", err)
	case !deleted:
		err = utils.ErrShadowConnectionNotFound
	}
	attempt.Complete(ctx, err)
	return err
}

func (s *shadowAgentService) TestConnection(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowConnectionTestInfo, error) {
	c, err := s.loadConnection(ctx, ouID, id)
	if err != nil {
		return nil, err
	}
	conn, err := s.runtimeConnection(c)
	if err != nil {
		return nil, err
	}
	connector, ok := s.connectors[c.Provider]
	if !ok {
		return nil, fmt.Errorf("no connector for provider %q", c.Provider)
	}
	tctx, cancel := context.WithTimeout(ctx, shadowConnectionTestTimeout)
	defer cancel()
	result := connector.Test(tctx, conn)
	for i := range result.Checks {
		result.Checks[i].Message = sa.ScrubText(result.Checks[i].Message)
	}
	if err := s.repo.SaveConnectionTest(ctx, ouID, id, s.now().UTC(), result); err != nil {
		return nil, fmt.Errorf("failed to save the test result: %w", err)
	}
	return &result, nil
}

// runtimeConnection decrypts a connection for a connector.
func (s *shadowAgentService) runtimeConnection(c *models.ShadowConnection) (sa.Connection, error) {
	return shadowRuntimeConnection(c, s.encryptionKey)
}

// shadowRuntimeConnection decrypts a connection's secret, for a connector.
func shadowRuntimeConnection(c *models.ShadowConnection, key []byte) (sa.Connection, error) {
	secret, err := decryptShadowSecret(c.SecretEncrypted, key)
	if err != nil {
		return sa.Connection{}, err
	}
	return sa.Connection{ID: c.ID, OUID: c.OUID, Name: c.Name, Provider: c.Provider, Config: c.Config, Secret: secret}, nil
}

func (s *shadowAgentService) encryptSecret(secret models.ShadowConnectionSecret) ([]byte, error) {
	plain, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("encoding the secret: %w", err)
	}
	out, err := utils.EncryptBytes(plain, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting the secret: %w", err)
	}
	return out, nil
}

func (s *shadowAgentService) decryptSecret(encrypted []byte) (models.ShadowConnectionSecret, error) {
	return decryptShadowSecret(encrypted, s.encryptionKey)
}

func decryptShadowSecret(encrypted, key []byte) (models.ShadowConnectionSecret, error) {
	var secret models.ShadowConnectionSecret
	plain, err := utils.DecryptBytes(encrypted, key)
	if err != nil {
		return secret, fmt.Errorf("decrypting the stored credential: %w", err)
	}
	if err := json.Unmarshal(plain, &secret); err != nil {
		return secret, fmt.Errorf("decoding the stored credential: %w", err)
	}
	return secret, nil
}

func validateConnectionBasics(in ShadowConnectionInput) (sa.ProviderSpec, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return sa.ProviderSpec{}, fmt.Errorf("%w: name is required", utils.ErrInvalidInput)
	}
	if len([]rune(name)) > maxShadowNameLength {
		return sa.ProviderSpec{}, fmt.Errorf("%w: name must be at most %d characters", utils.ErrInvalidInput, maxShadowNameLength)
	}
	spec, ok := sa.Provider(in.Provider)
	if !ok {
		return sa.ProviderSpec{}, fmt.Errorf("%w: unknown platform %q", utils.ErrInvalidInput, in.Provider)
	}
	return spec, nil
}

// resolveEndpoints merges the given endpoints over the current ones (or the
// official defaults) and validates every one.
func resolveEndpoints(spec sa.ProviderSpec, current, given map[string]string) (map[string]string, error) {
	known := map[string]sa.EndpointSpec{}
	for _, e := range spec.Endpoints {
		known[e.Key] = e
	}
	for k := range given {
		if base, value, ok := sa.SplitValueKey(k); ok {
			e, found := known[base]
			if !found || !e.PerValue() {
				return nil, fmt.Errorf("%w: unknown endpoint %q for %s", utils.ErrInvalidInput, k, spec.DisplayName)
			}
			if !sa.ValidPlaceValue(value) {
				return nil, fmt.Errorf("%w: %s: %q is not a valid %s", utils.ErrInvalidInput, e.Label, value, e.Placeholders[0])
			}
			continue
		}
		if _, ok := known[k]; !ok {
			return nil, fmt.Errorf("%w: unknown endpoint %q for %s", utils.ErrInvalidInput, k, spec.DisplayName)
		}
	}
	out := make(map[string]string, len(spec.Endpoints))
	for _, e := range spec.Endpoints {
		v := e.Default
		if cur, ok := current[e.Key]; ok && strings.TrimSpace(cur) != "" {
			v = cur
		}
		if g, ok := given[e.Key]; ok && strings.TrimSpace(g) != "" {
			v = strings.TrimSpace(g)
		}
		if err := sa.ValidateEndpoint(e, v); err != nil {
			return nil, fmt.Errorf("%w: %s", utils.ErrInvalidInput, err.Error())
		}
		out[e.Key] = strings.TrimRight(v, "/")
	}
	// One region's URL: kept from the stored connection unless given again;
	// a concrete URL, so no placeholder is allowed.
	regional := map[string]string{}
	for k, v := range current {
		if _, _, ok := sa.SplitValueKey(k); ok {
			regional[k] = v
		}
	}
	for k, v := range given {
		if _, _, ok := sa.SplitValueKey(k); ok {
			regional[k] = strings.TrimSpace(v)
		}
	}
	for k, v := range regional {
		base, value, _ := sa.SplitValueKey(k)
		e, found := known[base]
		if !found || v == "" {
			continue
		}
		concrete := e
		concrete.Placeholders = nil
		concrete.Label = e.Label + " (" + value + ")"
		if err := sa.ValidateEndpoint(concrete, v); err != nil {
			return nil, fmt.Errorf("%w: %s", utils.ErrInvalidInput, err.Error())
		}
		out[k] = strings.TrimRight(v, "/")
	}
	return out, nil
}

// pruneValueEndpoints drops the per-value URLs of values the connection no
// longer uses, and those equal to the template filled in (the official URL
// needs no override).
func pruneValueEndpoints(spec sa.ProviderSpec, cfg *models.ShadowConnectionConfig) {
	lists := shadowSettingsOf(spec.Provider, *cfg)
	for k, v := range cfg.Endpoints {
		base, value, ok := sa.SplitValueKey(k)
		if !ok {
			continue
		}
		var e *sa.EndpointSpec
		for i := range spec.Endpoints {
			if spec.Endpoints[i].Key == base {
				e = &spec.Endpoints[i]
			}
		}
		used := false
		if e != nil && e.PerValue() {
			candidates := e.FillValues
			if e.FillFrom != "" {
				candidates, _ = lists[e.FillFrom].([]string)
			}
			for _, c := range candidates {
				if strings.TrimSpace(c) == value && !slices.Contains(e.FillExclude, value) {
					used = true
				}
			}
		}
		filled := ""
		if e != nil {
			filled, _ = sa.Expand(cfg.Endpoints[base], map[string]string{e.Placeholders[0]: value})
		}
		if !used || filled == strings.TrimRight(v, "/") {
			delete(cfg.Endpoints, k)
		}
	}
}

// endpointWarnings lists endpoints whose host is not an official host of
// the provider. They are allowed; the UI warns.
func endpointWarnings(provider string, endpoints map[string]string) []string {
	spec, ok := sa.Provider(provider)
	if !ok {
		return []string{}
	}
	out := []string{}
	for _, e := range spec.Endpoints {
		if v, ok := endpoints[e.Key]; ok && !sa.IsOfficialHost(e, v) {
			out = append(out, fmt.Sprintf("%s (%s) is not an official %s host", e.Label, sa.Origin(strings.NewReplacer("{", "", "}", "").Replace(v)), spec.DisplayName))
		}
	}
	keys := make([]string, 0, len(endpoints))
	for k := range endpoints {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		base, value, ok := sa.SplitValueKey(k)
		if !ok {
			continue
		}
		for _, e := range spec.Endpoints {
			if e.Key == base && !sa.IsOfficialHost(e, endpoints[k]) {
				out = append(out, fmt.Sprintf("%s for %s (%s) is not an official %s host", e.Label, value, sa.Origin(endpoints[k]), spec.DisplayName))
			}
		}
	}
	return out
}

func connectionAuditOpts(ouID string, c *models.ShadowConnection, secretChanged bool) []audit.Option {
	opts := []audit.Option{
		audit.Org(ouID),
		audit.Detail("provider", c.Provider),
		audit.Detail("secretChanged", secretChanged),
	}
	if spec, ok := sa.Provider(c.Provider); ok && len(spec.Endpoints) > 0 {
		// Scheme and host only.
		ep := strings.NewReplacer("{", "", "}", "").Replace(c.Config.Endpoints[spec.Endpoints[0].Key])
		opts = append(opts, audit.Detail("endpoint", sa.Origin(ep)))
	}
	if c.ID != uuid.Nil {
		opts = append(opts, audit.ResourceNamed(audit.ResourceShadowConnection, c.ID.String(), c.Name))
	}
	return opts
}

func viewOfConnection(c *models.ShadowConnection) ShadowConnectionView {
	if c.SecretHints == nil {
		c.SecretHints = map[string]string{}
	}
	return ShadowConnectionView{
		Connection:       *c,
		Settings:         shadowSettingsOf(c.Provider, c.Config),
		EndpointWarnings: endpointWarnings(c.Provider, c.Config.Endpoints),
	}
}

// secretHints returns a redacted hint per stored secret field.
func secretHints(secret models.ShadowConnectionSecret) map[string]string {
	out := map[string]string{}
	add := func(k, v string) {
		if h := sa.SecretHint(v); h != "" {
			out[k] = h
		}
	}
	add("clientSecret", secret.ClientSecret)
	add("secretAccessKey", secret.SecretAccessKey)
	add("sessionToken", secret.SessionToken)
	add(sa.FieldPersonalAccessToken, secret.PersonalAccessToken)
	if strings.TrimSpace(secret.ServiceAccountJSON) != "" {
		out["serviceAccountJson"] = serviceAccountHint(secret.ServiceAccountJSON)
	}
	for id, v := range secret.ScopedKeys {
		add("scopedKeys."+id, v)
	}
	return out
}

// serviceAccountHint names the service account a key file is for, without
// any of its key material.
func serviceAccountHint(raw string) string {
	var f struct {
		ClientEmail  string `json:"client_email"`
		PrivateKeyID string `json:"private_key_id"`
	}
	if json.Unmarshal([]byte(raw), &f) != nil || f.ClientEmail == "" {
		return "••••"
	}
	id := f.PrivateKeyID
	if len(id) > 8 {
		id = id[:8] + "…"
	}
	return sa.Truncate(f.ClientEmail, 128) + " (key " + id + ")"
}

// ---- settings ----

// removeShadowSecrets clears the stored value of each named optional secret
// field. A required secret cannot be removed, and a field cannot be removed
// and given a new value in the same request.
func removeShadowSecrets(
	spec sa.ProviderSpec, keys []string, settings map[string]any, secret *models.ShadowConnectionSecret,
) error {
	for _, k := range keys {
		f, ok := spec.Field(k)
		if !ok || f.Kind != sa.FieldSecret {
			return fmt.Errorf("%w: %q is not a secret field of %s", utils.ErrInvalidInput, k, spec.DisplayName)
		}
		if f.Required {
			return fmt.Errorf("%w: %s is required and cannot be removed", utils.ErrInvalidInput, f.Label)
		}
		if v, given := settings[k].(string); given && strings.TrimSpace(v) != "" {
			return fmt.Errorf("%w: %s cannot be removed and set in the same request", utils.ErrInvalidInput, f.Label)
		}
		setShadowSecret(secret, k, "")
	}
	return nil
}

// checkCredentialChoice enforces a provider's "one of these" rule on the
// connection as it will be stored: a credential, or a ticked service.
func checkCredentialChoice(spec sa.ProviderSpec, secret models.ShadowConnectionSecret, cfg models.ShadowConnectionConfig) error {
	choice, ok := sa.CredentialChoiceOf(spec.Provider)
	if !ok {
		return nil
	}
	for _, k := range choice.Fields {
		if sa.IsServiceOption(k) {
			if sa.ServiceEnabled(cfg, k) {
				return nil
			}
			continue
		}
		if shadowSecretHas(secret, k) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", utils.ErrInvalidInput, choice.Message)
}

// shadowSecretFields are the secret's members, by form field key.
var shadowSecretFields = []string{
	sa.FieldScopedKeys, "clientSecret", "secretAccessKey", "sessionToken", "serviceAccountJson",
	sa.FieldPersonalAccessToken,
}

// pruneShadowSecret clears every secret member the provider's form has no
// field for, so a credential an earlier version of the form collected (e.g.
// the AWS key of the removed Anthropic cloud variants) is not kept.
func pruneShadowSecret(spec sa.ProviderSpec, secret *models.ShadowConnectionSecret) {
	for _, k := range shadowSecretFields {
		if _, ok := spec.Field(k); ok {
			continue
		}
		if k == sa.FieldScopedKeys {
			secret.ScopedKeys = nil
			continue
		}
		setShadowSecret(secret, k, "")
	}
}

// shadowSecretHas reports whether a secret field holds a credential; a key
// map holds one when at least one of its keys is set.
func shadowSecretHas(secret models.ShadowConnectionSecret, key string) bool {
	switch key {
	case sa.FieldScopedKeys:
		for _, v := range secret.ScopedKeys {
			if strings.TrimSpace(v) != "" {
				return true
			}
		}
		return false
	case "clientSecret":
		return strings.TrimSpace(secret.ClientSecret) != ""
	case "secretAccessKey":
		return strings.TrimSpace(secret.SecretAccessKey) != ""
	case "sessionToken":
		return strings.TrimSpace(secret.SessionToken) != ""
	case "serviceAccountJson":
		return strings.TrimSpace(secret.ServiceAccountJSON) != ""
	case sa.FieldPersonalAccessToken:
		return strings.TrimSpace(secret.PersonalAccessToken) != ""
	}
	return false
}

// applyShadowSettings maps a connection form's field values onto the
// connection's config and secret. Secret fields left empty keep the current
// secret unless create is true, in which case required ones are missing.
func applyShadowSettings(
	spec sa.ProviderSpec, settings map[string]any, cfg *models.ShadowConnectionConfig,
	secret *models.ShadowConnectionSecret, create bool,
) error {
	fields := map[string]sa.FieldSpec{}
	for _, f := range spec.Fields {
		fields[f.Key] = f
	}
	for k := range settings {
		if _, ok := fields[k]; !ok {
			return fmt.Errorf("%w: unknown setting %q for %s", utils.ErrInvalidInput, k, spec.DisplayName)
		}
	}
	// Checkboxes first: they decide which other fields apply.
	options := map[string]bool{}
	// switches are checkboxes that choose between two sets of fields (one
	// shown when ticked, the other with "!key" when not).
	switches := map[string]bool{}
	for _, f := range spec.Fields {
		if strings.HasPrefix(f.Option, "!") {
			switches[sa.OptionKey(f.Option)] = true
		}
	}
	for _, f := range spec.Fields {
		if f.Kind != sa.FieldCheckbox {
			continue
		}
		v := f.DefaultBool
		if raw, ok := settings[f.Key]; ok {
			b, ok := raw.(bool)
			if !ok {
				return fmt.Errorf("%w: %s must be true or false", utils.ErrInvalidInput, f.Label)
			}
			v = b
		}
		options[f.Key] = v
		setShadowBool(cfg, f.Key, v)
	}
	for _, f := range spec.Fields {
		if f.Kind == sa.FieldCheckbox {
			continue
		}
		active := sa.OptionActive(f.Option, options)
		// unused: the other side of an either/or switch, not kept.
		unused := !active && switches[sa.OptionKey(f.Option)]
		raw, given := settings[f.Key]
		switch f.Kind {
		case sa.FieldText:
			v, err := settingString(f, raw, given)
			if err != nil {
				return err
			}
			if unused {
				v = ""
			}
			if active && f.Required && v == "" {
				return fmt.Errorf("%w: %s is required", utils.ErrInvalidInput, f.Label)
			}
			setShadowText(cfg, f.Key, v)
		case sa.FieldList:
			v, err := settingList(f, raw, given)
			if err != nil {
				return err
			}
			if active && f.Required && len(v) == 0 {
				return fmt.Errorf("%w: %s needs at least one value", utils.ErrInvalidInput, f.Label)
			}
			setShadowList(cfg, f.Key, v)
		case sa.FieldSecret, sa.FieldTextarea:
			v, err := settingString(f, raw, given)
			if err != nil {
				return err
			}
			switch {
			case unused:
				// The other side of an either/or credential (Azure DevOps:
				// PAT or Entra): the connection no longer uses this secret,
				// so it is not kept.
				setShadowSecret(secret, f.Key, "")
			case v != "":
				if f.Key == "serviceAccountJson" && !json.Valid([]byte(v)) {
					return fmt.Errorf("%w: %s must be the JSON key file", utils.ErrInvalidInput, f.Label)
				}
				setShadowSecret(secret, f.Key, v)
			case active && f.Required && (create || !shadowSecretHas(*secret, f.Key)):
				// On update an empty value keeps the stored secret, unless
				// none is stored (e.g. Azure DevOps switching to a PAT).
				return fmt.Errorf("%w: %s is required", utils.ErrInvalidInput, f.Label)
			}
		case sa.FieldKeyMap:
			if !given || raw == nil {
				if secret.ScopedKeys != nil {
					cfg.ScopedKeyIDs = sortedKeys(secret.ScopedKeys)
				}
				continue
			}
			m, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: %s must map IDs to keys", utils.ErrInvalidInput, f.Label)
			}
			next := map[string]string{}
			for id, rv := range m {
				id = strings.TrimSpace(id)
				if id == "" {
					return fmt.Errorf("%w: %s has an empty ID", utils.ErrInvalidInput, f.Label)
				}
				val, ok := rv.(string)
				if !ok {
					return fmt.Errorf("%w: %s values must be strings", utils.ErrInvalidInput, f.Label)
				}
				val = strings.TrimSpace(val)
				if val == "" {
					// Keep the stored key for this ID.
					if old, ok := secret.ScopedKeys[id]; ok {
						val = old
					} else {
						return fmt.Errorf("%w: %s: a key is required for %s", utils.ErrInvalidInput, f.Label, id)
					}
				}
				next[id] = val
			}
			secret.ScopedKeys = next
			cfg.ScopedKeyIDs = sortedKeys(next)
			if active && f.Required && len(next) == 0 {
				return fmt.Errorf("%w: %s needs at least one ID and key", utils.ErrInvalidInput, f.Label)
			}
		}
	}
	for _, f := range spec.Fields {
		if f.Kind == sa.FieldKeyMap && f.Required && sa.OptionActive(f.Option, options) && !shadowSecretHas(*secret, f.Key) {
			return fmt.Errorf("%w: %s needs at least one ID and key", utils.ErrInvalidInput, f.Label)
		}
	}
	return nil
}

// GitHub names: an account login is alphanumeric with single hyphens, at
// most 39 characters; a repository name uses letters, digits, ".", "-" and
// "_", at most 100 characters.
var (
	githubOwnerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9]|-[A-Za-z0-9]){0,38}$`)
	githubRepoPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// validateSourceHostSettings checks the names a connection puts in API
// paths and hosts: Bedrock region codes, and GitHub and Azure DevOps names.
// A GitHub repository may be given as owner/name for the connection's
// owner; it is stored as the name.
func validateSourceHostSettings(spec sa.ProviderSpec, cfg *models.ShadowConnectionConfig) error {
	switch spec.Provider {
	case models.ShadowProviderBedrock:
		for _, r := range cfg.Regions {
			if !sa.ValidRegion(r) {
				return fmt.Errorf("%w: %q is not an AWS region code, such as us-east-1", utils.ErrInvalidInput, r)
			}
		}
	case models.ShadowProviderGitHub:
		if !githubOwnerPattern.MatchString(cfg.Organization) {
			return fmt.Errorf("%w: Organization or user account must be a GitHub account name, such as my-org", utils.ErrInvalidInput)
		}
		for i, r := range cfg.Repositories {
			name := r
			if owner, n, ok := strings.Cut(r, "/"); ok {
				if !strings.EqualFold(owner, cfg.Organization) {
					return fmt.Errorf("%w: repository %q is not in %s", utils.ErrInvalidInput, r, cfg.Organization)
				}
				name = n
			}
			if !githubRepoPattern.MatchString(name) {
				return fmt.Errorf("%w: %q is not a GitHub repository name", utils.ErrInvalidInput, r)
			}
			cfg.Repositories[i] = name
		}
	case models.ShadowProviderAzureDevOps:
		if strings.ContainsAny(cfg.Organization, "/\\?#% \t") {
			return fmt.Errorf("%w: Organization must be the name in https://dev.azure.com/{organization}, not a URL", utils.ErrInvalidInput)
		}
		for _, p := range cfg.Projects {
			if strings.ContainsAny(p, "/\\?#%") {
				return fmt.Errorf("%w: %q is not an Azure DevOps project name", utils.ErrInvalidInput, p)
			}
		}
	}
	return nil
}

func settingString(f sa.FieldSpec, raw any, given bool) (string, error) {
	if !given || raw == nil {
		return "", nil
	}
	v, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be text", utils.ErrInvalidInput, f.Label)
	}
	return strings.TrimSpace(v), nil
}

func settingList(f sa.FieldSpec, raw any, given bool) ([]string, error) {
	if !given || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be a list", utils.ErrInvalidInput, f.Label)
	}
	out := []string{}
	seen := map[string]bool{}
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s must be a list of text values", utils.ErrInvalidInput, f.Label)
		}
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out, nil
}

func setShadowBool(cfg *models.ShadowConnectionConfig, key string, v bool) {
	switch key {
	case sa.OptAllSubscriptions:
		cfg.AllSubscriptions = v
	case sa.OptClassicAgents:
		cfg.IncludeClassicAgents = v
	case sa.OptGlobalMulti:
		cfg.IncludeGlobalAndMultiRegion = v
	case sa.OptAllRepositories:
		cfg.AllRepositories = v
	case sa.OptAllProjects:
		cfg.AllProjects = v
	case sa.OptUseEntra:
		cfg.UseEntra = v
	case sa.OptReadCIAndEnv:
		cfg.ReadCIAndEnvFiles = v
	default:
		if !sa.IsServiceOption(key) {
			return
		}
		kept := cfg.DisabledServices[:0:0]
		for _, k := range cfg.DisabledServices {
			if k != key {
				kept = append(kept, k)
			}
		}
		if !v {
			kept = append(kept, key)
		}
		cfg.DisabledServices = kept
	}
}

func setShadowText(cfg *models.ShadowConnectionConfig, key, v string) {
	switch key {
	case "tenantId":
		cfg.TenantID = v
	case "clientId":
		cfg.ClientID = v
	case "roleArn":
		cfg.RoleARN = v
	case "externalId":
		cfg.ExternalID = v
	case "accessKeyId":
		cfg.AccessKeyID = v
	case "organization":
		cfg.Organization = v
	}
}

func setShadowList(cfg *models.ShadowConnectionConfig, key string, v []string) {
	switch key {
	case "subscriptionIds":
		cfg.SubscriptionIDs = v
	case "regions":
		cfg.Regions = v
	case "projectIds":
		cfg.ProjectIDs = v
	case "locations":
		cfg.Locations = v
	case "repositories":
		cfg.Repositories = v
	case "projects":
		cfg.Projects = v
	}
}

func setShadowSecret(secret *models.ShadowConnectionSecret, key, v string) {
	switch key {
	case "clientSecret":
		secret.ClientSecret = v
	case "secretAccessKey":
		secret.SecretAccessKey = v
	case "sessionToken":
		secret.SessionToken = v
	case "serviceAccountJson":
		secret.ServiceAccountJSON = v
	case sa.FieldPersonalAccessToken:
		secret.PersonalAccessToken = v
	}
}

// shadowSettingsOf returns a connection's non-secret field values, keyed as
// the provider's form expects them.
func shadowSettingsOf(provider string, cfg models.ShadowConnectionConfig) map[string]any {
	spec, ok := sa.Provider(provider)
	out := map[string]any{}
	if !ok {
		return out
	}
	text := map[string]string{
		"tenantId": cfg.TenantID, "clientId": cfg.ClientID, "roleArn": cfg.RoleARN, "externalId": cfg.ExternalID,
		"accessKeyId": cfg.AccessKeyID, "organization": cfg.Organization,
	}
	lists := map[string][]string{
		"subscriptionIds": cfg.SubscriptionIDs, "regions": cfg.Regions, "projectIds": cfg.ProjectIDs,
		"locations":    cfg.Locations,
		"repositories": cfg.Repositories, "projects": cfg.Projects,
	}
	bools := map[string]bool{
		sa.OptAllSubscriptions: cfg.AllSubscriptions,
		sa.OptClassicAgents:    cfg.IncludeClassicAgents, sa.OptGlobalMulti: cfg.IncludeGlobalAndMultiRegion,
		sa.OptAllRepositories: cfg.AllRepositories,
		sa.OptAllProjects:     cfg.AllProjects, sa.OptUseEntra: cfg.UseEntra, sa.OptReadCIAndEnv: cfg.ReadCIAndEnvFiles,
	}
	for _, f := range spec.Fields {
		switch f.Kind {
		case sa.FieldText:
			out[f.Key] = text[f.Key]
		case sa.FieldList:
			if v := lists[f.Key]; v != nil {
				out[f.Key] = v
			} else {
				out[f.Key] = []string{}
			}
		case sa.FieldCheckbox:
			if sa.IsServiceOption(f.Key) {
				out[f.Key] = sa.ServiceEnabled(cfg, f.Key)
			} else {
				out[f.Key] = bools[f.Key]
			}
		case sa.FieldKeyMap:
			ids := cfg.ScopedKeyIDs
			if ids == nil {
				ids = []string{}
			}
			out[f.Key+"Ids"] = ids
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- scans ----

func (s *shadowAgentService) CreateScan(
	ctx context.Context, ouID string, connectionIDs []uuid.UUID,
) (*models.ShadowScan, error) {
	all, err := s.repo.ListConnections(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	byID := map[uuid.UUID]bool{}
	for i := range all {
		byID[all[i].ID] = true
	}
	ids := []string{}
	if len(connectionIDs) == 0 {
		for i := range all {
			ids = append(ids, all[i].ID.String())
		}
	} else {
		seen := map[uuid.UUID]bool{}
		for _, id := range connectionIDs {
			if !byID[id] {
				return nil, fmt.Errorf("%w: connection %s not found", utils.ErrInvalidInput, id)
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id.String())
			}
		}
	}
	if len(ids) == 0 {
		return nil, utils.ErrShadowNoConnections
	}
	scan := &models.ShadowScan{
		ID:            uuid.New(),
		OUID:          ouID,
		Status:        models.ShadowScanQueued,
		ConnectionIDs: ids,
		Results:       []models.ShadowScanConnectionResult{},
		RequestedBy:   audit.BuildEvent(ctx, audit.ActionShadowScanCreate).ActorID,
	}
	if err := s.repo.CreateScan(ctx, scan); err != nil {
		if errors.Is(err, utils.ErrShadowScanInProgress) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to queue the scan: %w", err)
	}
	return scan, nil
}

func (s *shadowAgentService) ListScans(ctx context.Context, ouID string, limit int) ([]models.ShadowScan, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	list, err := s.repo.ListScans(ctx, ouID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list scans: %w", err)
	}
	return list, nil
}

func (s *shadowAgentService) ClearScans(ctx context.Context, ouID string) (int, error) {
	n, err := s.repo.DeleteFinishedScans(ctx, ouID)
	if err != nil {
		return 0, fmt.Errorf("failed to clear scans: %w", err)
	}
	return int(n), nil
}

func (s *shadowAgentService) GetScan(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowScan, error) {
	scan, err := s.repo.GetScan(ctx, ouID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrShadowScanNotFound
		}
		return nil, fmt.Errorf("failed to get scan: %w", err)
	}
	return scan, nil
}

func (s *shadowAgentService) Summary(ctx context.Context, ouID string) (repositories.ShadowSummary, *models.ShadowScan, error) {
	sum, err := s.repo.Summary(ctx, ouID)
	if err != nil {
		return sum, nil, fmt.Errorf("failed to summarize agents: %w", err)
	}
	latest, err := s.repo.LatestScan(ctx, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sum, nil, nil
		}
		return sum, nil, fmt.Errorf("failed to get the latest scan: %w", err)
	}
	return sum, latest, nil
}

// ---- findings ----

var shadowFindingStatuses = map[string]bool{
	models.ShadowStatusNew: true, models.ShadowStatusReviewed: true, models.ShadowStatusDismissed: true,
}

func (s *shadowAgentService) ListFindings(
	ctx context.Context, ouID string, f repositories.ShadowFindingFilter,
) ([]repositories.ShadowFindingRow, int, error) {
	if f.Provider != "" {
		if _, ok := sa.Provider(f.Provider); !ok {
			return nil, 0, fmt.Errorf("%w: unknown platform %q", utils.ErrInvalidInput, f.Provider)
		}
	}
	if f.Status != "" && !shadowFindingStatuses[f.Status] {
		return nil, 0, fmt.Errorf("%w: unknown status %q", utils.ErrInvalidInput, f.Status)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	rows, total, err := s.repo.ListFindings(ctx, ouID, f)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list agents: %w", err)
	}
	return rows, total, nil
}

func (s *shadowAgentService) GetFinding(ctx context.Context, ouID string, id uuid.UUID) (*ShadowFindingView, error) {
	row, err := s.repo.GetFinding(ctx, ouID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrShadowFindingNotFound
		}
		return nil, fmt.Errorf("failed to get the agent: %w", err)
	}
	signals, err := s.repo.ListSignals(ctx, ouID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list the agent's evidence: %w", err)
	}
	related, err := s.relatedViews(ctx, ouID, row.Related)
	if err != nil {
		return nil, err
	}
	return &ShadowFindingView{Row: *row, Signals: signals, Related: related}, nil
}

func (s *shadowAgentService) relatedViews(
	ctx context.Context, ouID string, rels []models.ShadowFindingRelation,
) ([]ShadowRelationView, error) {
	out := make([]ShadowRelationView, 0, len(rels))
	for _, rel := range rels {
		v := ShadowRelationView{ShadowFindingRelation: rel}
		if id, err := uuid.Parse(rel.FindingID); err == nil {
			other, err := s.repo.GetFinding(ctx, ouID, id)
			switch {
			case err == nil:
				v.DisplayName, v.Provider = other.DisplayName, other.Provider
			case errors.Is(err, gorm.ErrRecordNotFound):
				continue // the related finding was deleted with its connection
			default:
				return nil, fmt.Errorf("failed to read a related finding: %w", err)
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *shadowAgentService) DeleteFinding(ctx context.Context, ouID string, id uuid.UUID) error {
	found, err := s.repo.DeleteFinding(ctx, ouID, id)
	if err != nil {
		return fmt.Errorf("failed to delete the agent: %w", err)
	}
	if !found {
		return utils.ErrShadowFindingNotFound
	}
	return nil
}

func (s *shadowAgentService) DeleteFindings(ctx context.Context, ouID string) (int, error) {
	n, err := s.repo.DeleteFindings(ctx, ouID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete the agents: %w", err)
	}
	return int(n), nil
}

func (s *shadowAgentService) UpdateFindingStatus(ctx context.Context, ouID string, id uuid.UUID, status string) (*ShadowFindingView, error) {
	switch status {
	case models.ShadowStatusNew, models.ShadowStatusReviewed, models.ShadowStatusDismissed:
	default:
		return nil, fmt.Errorf("%w: status must be new, reviewed or dismissed", utils.ErrInvalidInput)
	}
	if err := s.repo.UpdateFindingStatus(ctx, ouID, id, status); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrShadowFindingNotFound
		}
		return nil, fmt.Errorf("failed to update the agent: %w", err)
	}
	return s.GetFinding(ctx, ouID, id)
}
