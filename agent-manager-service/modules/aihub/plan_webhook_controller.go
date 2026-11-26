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

package aihub

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha3"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
)

// The webhook receiver for subscription plans made, changed or deleted elsewhere (including an MCP
// hub), so those plans are copied into Agent Manager:
//
//   - POST /m/ai-hub/webhooks/events on the internal server (no user sign-in: the request is
//     authenticated by its signature);
//   - a signature header "t=<unix seconds>,v1=<hex HMAC-SHA256 of "<t>.<raw body>">", with the
//     timestamp within the tolerance (replay guard);
//   - the envelope's secret_hash, the hex SHA3-256 of "<label>:<secret>", names the secret that signed
//     the delivery; it is only trusted once the signature verifies;
//   - org.ref_id is the organization's handle;
//   - events subscription_plan.created / .updated (both upsert) and subscription_plan.deleted;
//   - events subscription.created / .updated / .plan_changed / .token_regenerated / .deleted, for
//     subscriptions to MCP servers (subscription_service.go); subscription.billing_updated is accepted
//     and changes nothing.
//
// A delivery the sender got wrong (signature, envelope, organization, event) is answered alike (401), so a
// caller learns nothing from the response; a failure on this side (the database, publishing the change, a
// panic) answers 503: the delivery wasn't applied (the developer portal sends each delivery once). The reason is logged. The secret is the module setting webhook_secret (MODULE_AI_HUB_WEBHOOK_SECRET).

// webhookPath is the receiver's route below the module's webhooks (Runtime.HandleWebhook).
const webhookPath = "/events"

// WebhookRoutePath is the receiver's path on the internal server, the URL path a developer portal's
// subscriber is configured with.
const WebhookRoutePath = "/m/" + Name + "/webhooks" + webhookPath

// Subscription plan event types.
const (
	EventSubscriptionPlanCreated = "subscription_plan.created"
	EventSubscriptionPlanUpdated = "subscription_plan.updated"
	EventSubscriptionPlanDeleted = "subscription_plan.deleted"
)

// Subscription event types. The event's data names the subscription, its artifact (api) and plan, and
// carries the subscription key sealed in its token field (encrypted_fields).
const (
	EventSubscriptionCreated          = "subscription.created"
	EventSubscriptionUpdated          = "subscription.updated"
	EventSubscriptionPlanChanged      = "subscription.plan_changed"
	EventSubscriptionTokenRegenerated = "subscription.token_regenerated"
	EventSubscriptionDeleted          = "subscription.deleted"
	EventSubscriptionBillingUpdated   = "subscription.billing_updated"
)

const (
	// webhookSecretHashLabel and DefaultWebhookSignatureHeader are part of the delivery protocol:
	// they must stay byte-identical to the sender's. The header can be changed with the setting
	// webhook_signature_header.
	webhookSecretHashLabel = "api-portal-webhook-secret-hash-v1"

	DefaultWebhookSignatureHeader = "X-Api-Portal-Signature"
	// DefaultWebhookSourceName is the origin of the plans the webhook copies in, unless webhook_source_name names
	// the developer portal.
	DefaultWebhookSourceName  = "developer-portal"
	defaultSignatureTolerance = 5 * time.Minute
	defaultMaxBodyBytes       = 1 << 20 // 1 MiB
)

var (
	errSignatureMissing        = errors.New("webhook signature header missing")
	errSignatureInvalid        = errors.New("webhook signature invalid")
	errTimestampOutOfTolerance = errors.New("webhook signature timestamp outside tolerance")
	errInvalidEnvelope         = errors.New("invalid webhook envelope")
	errUnsupportedEvent        = errors.New("unsupported webhook event type")
	errOrganizationNotFound    = errors.New("organization not found")
	// errWebhookDisabled refuses every delivery while no webhook_secret is set: a retry can't succeed.
	errWebhookDisabled = errors.New("the subscription plan webhook is disabled (no webhook_secret is set)")
)

// webhookConfig is the receiver's configuration, from the module's settings.
type webhookConfig struct {
	// Enabled registers nothing new: with it off (the default when no secret is set), every delivery
	// is refused.
	Enabled bool
	// Secret is the secret shared with the sender. It signs each delivery.
	Secret string
	// SignatureHeader carries "t=...,v1=...".
	SignatureHeader string
	// SignatureTolerance bounds how old a signed delivery may be.
	SignatureTolerance time.Duration
	// MaxBodyBytes caps the request body.
	MaxBodyBytes int64
	// LegacySharedSecret accepts a delivery with no secret_hash (a sender that predates it), verified
	// against Secret. Off by default.
	LegacySharedSecret bool
	// SourceName names the developer portal that sends the deliveries: the origin of the plans it creates.
	SourceName string
}

// webhookConfigFrom reads the module settings (amp.yaml modules[].config, or MODULE_AI_HUB_<KEY>).
func webhookConfigFrom(get func(key string) string, log *slog.Logger) webhookConfig {
	cfg := webhookConfig{
		Secret:             strings.TrimSpace(get("webhook_secret")),
		SignatureHeader:    DefaultWebhookSignatureHeader,
		SignatureTolerance: defaultSignatureTolerance,
		MaxBodyBytes:       defaultMaxBodyBytes,
		SourceName:         DefaultWebhookSourceName,
	}
	if v := strings.TrimSpace(get("webhook_source_name")); v != "" {
		cfg.SourceName = v
	}
	cfg.Enabled = cfg.Secret != ""
	if v := strings.TrimSpace(get("webhook_enabled")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Enabled = b && cfg.Secret != ""
		}
	}
	if v := strings.TrimSpace(get("webhook_signature_header")); v != "" {
		cfg.SignatureHeader = v
	}
	if v := strings.TrimSpace(get("webhook_signature_tolerance")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SignatureTolerance = d
		} else {
			log.Warn("ignoring an invalid webhook_signature_tolerance", "value", v)
		}
	}
	if v := strings.TrimSpace(get("webhook_max_body_size")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.MaxBodyBytes = n
		}
	}
	if v := strings.TrimSpace(get("webhook_legacy_shared_secret")); v != "" {
		cfg.LegacySharedSecret, _ = strconv.ParseBool(v)
	}
	return cfg
}

// SecretHash returns the identifier a sender puts in the envelope's secret_hash: the lowercase
// hex SHA3-256 of "<label>:<secret>". One-way; not key material.
func SecretHash(secret string) string {
	sum := sha3.Sum256([]byte(webhookSecretHashLabel + ":" + secret))
	return hex.EncodeToString(sum[:])
}

// Sign returns the signature header value for body at t, as a sender computes it. It is used
// by tests and by anything that needs to send the receiver a delivery.
func Sign(secret string, body []byte, t time.Time) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(body)))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// verifySignature checks a "t=...,v1=..." header against the raw body.
func verifySignature(secret, header string, body []byte, tolerance time.Duration, now time.Time) error {
	if strings.TrimSpace(header) == "" {
		return errSignatureMissing
	}
	var tsStr, sigStr string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "t":
			tsStr = strings.TrimSpace(kv[1])
		case "v1":
			sigStr = strings.TrimSpace(kv[1])
		}
	}
	if tsStr == "" || sigStr == "" {
		return fmt.Errorf("%w: malformed signature header", errSignatureInvalid)
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid timestamp", errSignatureInvalid)
	}
	// Stale or future-dated signatures are refused (replay guard).
	if tolerance > 0 {
		delta := now.Sub(time.Unix(ts, 0))
		if delta < 0 {
			delta = -delta
		}
		if delta > tolerance {
			return errTimestampOutOfTolerance
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsStr + "." + string(body)))
	got, err := hex.DecodeString(sigStr)
	if err != nil || !hmac.Equal(mac.Sum(nil), got) {
		return errSignatureInvalid
	}
	return nil
}

// envelope is the webhook envelope every delivery carries.
type envelope struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	OccurredAt string `json:"occurred_at"`
	Org        struct {
		RefID string `json:"ref_id"`
	} `json:"org"`
	// OrgID is the legacy flat org identifier, superseded by org.ref_id.
	OrgID         string          `json:"org_id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	SchemaVersion string          `json:"schema_version"`
	SecretHash    string          `json:"secret_hash"`
	Data          json.RawMessage `json:"data"`
}

func decodeEnvelope(body []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidEnvelope, err)
	}
	if env.Org.RefID != "" {
		env.OrgID = env.Org.RefID
	}
	return &env, nil
}

func (e *envelope) validate() error {
	switch {
	case e.EventID == "":
		return fmt.Errorf("%w: event_id is required", errInvalidEnvelope)
	case e.EventType == "":
		return fmt.Errorf("%w: event_type is required", errInvalidEnvelope)
	case e.OrgID == "":
		return fmt.Errorf("%w: org.ref_id is required", errInvalidEnvelope)
	}
	return nil
}

// occurredAt is when the change was made at the source, or now when the envelope doesn't say.
func (e *envelope) occurredAt() time.Time {
	value := strings.TrimSpace(e.OccurredAt)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// planEventData is the data of subscription_plan.* events. handle is the only field on .deleted.
type planEventData struct {
	Handle      string           `json:"handle"`
	DisplayName string           `json:"display_name"`
	Description string           `json:"description"`
	Limits      []webhookLimit   `json:"limits"`
	Conditions  *[]Condition     `json:"conditions"` // absent from senders that predate conditions
	Pricing     *json.RawMessage `json:"pricing"`    // null for a free plan
}

// webhookLimit is one plan limit, as older senders describe it. limit_count -1 means unlimited.
type webhookLimit struct {
	LimitType  string `json:"limit_type"`
	LimitCount int    `json:"limit_count"`
	TimeUnit   string `json:"time_unit"`
	TimeAmount int    `json:"time_amount"`
}

// requestWindowsFrom maps REQUEST_COUNT limits to request-count windows. Unlimited,
// unsupported and repeated-length limits are skipped; a plan holds at most ten windows.
func requestWindowsFrom(limits []webhookLimit) []Window {
	var windows []Window
	seen := map[string]bool{}
	for _, l := range limits {
		if !strings.EqualFold(l.LimitType, "REQUEST_COUNT") || l.LimitCount < 1 {
			continue
		}
		unit := strings.ToUpper(strings.TrimSpace(l.TimeUnit))
		if !timeUnits[unit] {
			continue
		}
		amount := max(l.TimeAmount, 1)
		key := windowKey(amount, unit)
		if seen[key] || len(windows) == maxConditionWindows {
			continue
		}
		seen[key] = true
		windows = append(windows, Window{Count: l.LimitCount, TimeAmount: amount, TimeUnit: unit})
	}
	return windows
}

func (d *planEventData) requireHandle() error {
	d.Handle = strings.TrimSpace(d.Handle)
	if d.Handle == "" {
		return fmt.Errorf("%w: data.handle is required", errInvalidEnvelope)
	}
	if utf8.RuneCountInString(d.Handle) > maxHandleChars {
		return fmt.Errorf("%w: data.handle must be at most %d characters", errInvalidEnvelope, maxHandleChars)
	}
	return nil
}

// webhookReceiver receives the deliveries for subscription plans.
type webhookReceiver struct {
	cfg   webhookConfig
	plans *planService
	// subscriptions copies in the subscriptions made at the portal.
	subscriptions *subscriptionService
	core          modkit.CoreAPI
	log           *slog.Logger
	now           func() time.Time
}

// webhookCaller is how the audit record names the sender: no user signs in, the signature authenticates it.
func webhookCaller(refused string, signed bool) modkit.InternalCaller {
	return modkit.InternalCaller{
		Kind: "developer-portal", AuthMethod: "webhook-signature",
		Refused: refused, CredentialPresent: signed,
	}
}

// ServeHTTP runs the flow: size-limited read, signature check, envelope, organization, dispatch.
// The sender's faults are answered alike (401), except a signed request the plans here refuse (409, refusal); ours
// answer 503: the delivery wasn't applied. The reason is logged.
func (r *webhookReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			r.log.Error("webhook delivery panicked", "panic", rec)
			unavailable(w)
		}
	}()
	if err := r.receive(w, req); err != nil {
		level := slog.LevelWarn
		if !isClientFault(err) {
			level = slog.LevelError
		}
		r.log.Log(req.Context(), level, "webhook delivery rejected", "error", err, "clientIp", req.RemoteAddr)
		if errors.Is(err, errSignatureInvalid) {
			modkit.AuditInternalCaller(req.Context(),
				webhookCaller("invalid-signature", req.Header.Get(r.cfg.SignatureHeader) != ""))
		}
		if refused := refusal(err); refused != nil {
			writeJSONStatus(w, http.StatusConflict, refused)
			return
		}
		if !isClientFault(err) {
			unavailable(w)
			return
		}
		reject(w)
	}
}

// Codes of a signed delivery refused for what it asks (409), which a retry can't change.
const (
	codePlanDeprecated     = "SUBSCRIPTION_PLAN_DEPRECATED"
	codePlanOriginConflict = "SUBSCRIPTION_PLAN_ORIGIN_CONFLICT"
)

// refusal answers a signed delivery that asks for what the plans here don't allow: subscribing to a deprecated
// plan, or creating a plan with the handle of one made elsewhere. The sender is authenticated by then, so it is
// told why, and a 409 is not retried. Nil for any other error.
func refusal(err error) *errorResponse {
	switch {
	case errors.Is(err, errPlanDeprecated):
		return &errorResponse{
			Status: "error", Code: codePlanDeprecated,
			Message: "The subscription plan is deprecated: it takes no new subscriptions and can't be created again.",
		}
	case errors.Is(err, errPlanOriginConflict):
		return &errorResponse{
			Status: "error", Code: codePlanOriginConflict,
			Message: "A subscription plan with this handle was made elsewhere; create the plan with another handle.",
		}
	default:
		return nil
	}
}

func (r *webhookReceiver) receive(w http.ResponseWriter, req *http.Request) error {
	if !r.cfg.Enabled {
		return errWebhookDisabled
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, r.cfg.MaxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: body too large or unreadable: %w", errInvalidEnvelope, err)
	}
	// 1. The signature, over the raw body, with the secret the envelope names.
	if err := r.authenticate(req.Header.Get(r.cfg.SignatureHeader), body); err != nil {
		return err
	}
	modkit.AuditInternalCaller(req.Context(), webhookCaller("", true))
	// 2. The envelope.
	env, err := decodeEnvelope(body)
	if err == nil {
		err = env.validate()
	}
	if err != nil {
		return err
	}
	log := r.log.With("eventId", env.EventID, "eventType", env.EventType, "orgHandle", env.OrgID)
	// 3. The organization, by its handle.
	orgID, err := r.orgIDByHandle(req.Context(), env.OrgID)
	if err != nil {
		return err
	}
	// 4. The event. Handlers are idempotent by the plan's handle, so a repeated delivery is safe.
	if err := r.dispatch(req.Context(), orgID, env); err != nil {
		return fmt.Errorf("event %s %s: %w", env.EventType, env.EventID, err)
	}
	log.Info("webhook event processed", "orgId", orgID)
	writeJSONStatus(w, http.StatusOK, map[string]string{"status": "processed"})
	return nil
}

// authenticate verifies the signature. A delivery naming a secret_hash is checked against exactly
// the secret with that hash, and refused when no such secret is held. The envelope is read before
// anything is verified, so its fields only select the secret and are never trusted.
func (r *webhookReceiver) authenticate(header string, body []byte) error {
	env, err := decodeEnvelope(body)
	if err != nil {
		env = &envelope{}
	}
	if env.SecretHash != "" {
		if env.SecretHash != SecretHash(r.cfg.Secret) {
			return fmt.Errorf("%w: unknown secret_hash", errSignatureInvalid)
		}
	} else if !r.cfg.LegacySharedSecret {
		return fmt.Errorf("%w: the delivery names no secret_hash", errSignatureInvalid)
	}
	return verifySignature(r.cfg.Secret, header, body, r.cfg.SignatureTolerance, r.now())
}

// orgIDByHandle resolves the organization handle a delivery names (org.ref_id) to its id, through the core.
func (r *webhookReceiver) orgIDByHandle(ctx context.Context, handle string) (string, error) {
	org, err := r.core.GetOrgByHandle(ctx, handle)
	if errors.Is(err, modkit.ErrNotFound) {
		return "", fmt.Errorf("%w: %s", errOrganizationNotFound, handle)
	}
	if err != nil {
		return "", fmt.Errorf("look up organization %s: %w", handle, err)
	}
	return org.ID, nil
}

func (r *webhookReceiver) dispatch(ctx context.Context, orgID string, env *envelope) error {
	switch env.EventType {
	case EventSubscriptionCreated, EventSubscriptionUpdated, EventSubscriptionPlanChanged,
		EventSubscriptionTokenRegenerated, EventSubscriptionDeleted:
		ch, err := r.subscriptionChange(env)
		if err != nil {
			return err
		}
		return r.subscriptions.applyFromWebhook(ctx, orgID, ch)
	case EventSubscriptionBillingUpdated:
		// Plans here are free: billing doesn't change a subscription.
		return nil
	case EventSubscriptionPlanCreated, EventSubscriptionPlanUpdated:
		d, err := planData(env)
		if err != nil {
			return err
		}
		name := strings.TrimSpace(d.DisplayName)
		if name == "" {
			return fmt.Errorf("%w: data.display_name is required", errInvalidEnvelope)
		}
		paid := d.Pricing != nil && string(*d.Pricing) != "null"
		return r.plans.upsertFromWebhook(ctx, orgID, r.cfg.SourceName, d.Handle, name, d.Description, d.Conditions,
			requestWindowsFrom(d.Limits), paid, env.EventType == EventSubscriptionPlanCreated, env.occurredAt())
	case EventSubscriptionPlanDeleted:
		d, err := planData(env)
		if err != nil {
			return err
		}
		return r.plans.deleteFromWebhook(ctx, orgID, d.Handle, env.occurredAt())
	default:
		return fmt.Errorf("%w: %s", errUnsupportedEvent, env.EventType)
	}
}

// subscriptionEventData is the data of subscription.* events.
type subscriptionEventData struct {
	SubscriptionID string `json:"subscription_id"`
	SubscriberID   string `json:"subscriber_id"`
	Status         string `json:"status"`
	Plan           struct {
		RefID string `json:"ref_id"`
	} `json:"subscription_plan"`
	API struct {
		RefID string `json:"ref_id"`
		Type  string `json:"type"`
	} `json:"api"`
	Token json.RawMessage `json:"token"`
}

// subscriptionChange reads a subscription event: its artifact must be an MCP server (api.type Mcp, api.ref_id
// its handle here), its plan a plan handle (subscription_plan.ref_id), and a token, when present, sealed.
func (r *webhookReceiver) subscriptionChange(env *envelope) (subscriptionChange, error) {
	if len(env.Data) == 0 {
		return subscriptionChange{}, fmt.Errorf("%w: data is required for event %s", errInvalidEnvelope, env.EventType)
	}
	var d subscriptionEventData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return subscriptionChange{}, fmt.Errorf("%w: invalid data for event %s: %w", errInvalidEnvelope, env.EventType, err)
	}
	ch := subscriptionChange{
		Event:          strings.TrimPrefix(env.EventType, "subscription."),
		At:             env.occurredAt(),
		SourceID:       strings.TrimSpace(d.SubscriptionID),
		SubscriberID:   strings.TrimSpace(d.SubscriberID),
		ArtifactKind:   strings.TrimSpace(d.API.Type),
		ArtifactHandle: strings.TrimSpace(d.API.RefID),
		PlanHandle:     strings.TrimSpace(d.Plan.RefID),
		Status:         strings.TrimSpace(d.Status),
	}
	switch {
	case ch.SourceID == "":
		return ch, fmt.Errorf("%w: data.subscription_id is required", errInvalidEnvelope)
	case ch.Event == "deleted":
		return ch, nil
	case ch.ArtifactKind != subplanv1.ArtifactKindMCP:
		return ch, fmt.Errorf("%w: only MCP servers take subscriptions here, not %q", errUnsupportedEvent, ch.ArtifactKind)
	case ch.ArtifactHandle == "":
		return ch, fmt.Errorf("%w: data.api.ref_id is required", errInvalidEnvelope)
	case ch.PlanHandle == "" && (ch.Event == "created" || ch.Event == "plan_changed"):
		return ch, fmt.Errorf("%w: data.subscription_plan.ref_id is required", errInvalidEnvelope)
	}
	if len(d.Token) > 0 && string(d.Token) != "null" {
		token, err := decryptField(r.cfg.Secret, d.Token)
		if err != nil {
			return ch, err
		}
		ch.Token = &token
	}
	return ch, nil
}

func planData(env *envelope) (*planEventData, error) {
	if len(env.Data) == 0 {
		return nil, fmt.Errorf("%w: data is required for event %s", errInvalidEnvelope, env.EventType)
	}
	var d planEventData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return nil, fmt.Errorf("%w: invalid data for event %s: %w", errInvalidEnvelope, env.EventType, err)
	}
	return &d, d.requireHandle()
}

// isClientFault reports whether a rejection was the sender's doing (401, logged at WARN) rather than ours
// (503, logged at ERROR).
func isClientFault(err error) bool {
	var v *validationError
	return errors.Is(err, errSignatureMissing) || errors.Is(err, errSignatureInvalid) ||
		errors.Is(err, errTimestampOutOfTolerance) || errors.Is(err, errInvalidEnvelope) ||
		errors.Is(err, errUnsupportedEvent) || errors.Is(err, errOrganizationNotFound) ||
		errors.Is(err, errWebhookDisabled) || errors.Is(err, errSubscriptionPlanUnknown) || errors.As(err, &v) ||
		errors.Is(err, errPlanDeprecated) || errors.Is(err, errPlanOriginConflict)
}

// reject answers a refused delivery: always 401, with nothing about why.
func reject(w http.ResponseWriter) {
	writeJSONStatus(w, http.StatusUnauthorized, errorResponse{
		Status: "error", Code: codeUnauthorized,
		Message: "Invalid or expired credentials.",
	})
}

// unavailable answers a delivery that failed on this side: 503, with nothing about why.
func unavailable(w http.ResponseWriter) {
	writeJSONStatus(w, http.StatusServiceUnavailable, errorResponse{
		Status: "error", Code: codeUnavailable,
		Message: "The delivery could not be processed. Retry later.",
	})
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
