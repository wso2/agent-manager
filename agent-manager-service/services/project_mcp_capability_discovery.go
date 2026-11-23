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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

// A project MCP proxy binding whose caller sent no capabilities has them discovered by AMS: once the binding is
// deployed (the MCP kind's AfterLive queues it, and a refresh queues it again), the reconciler lists the MCP server's
// tools, prompts and resources and stores them on the binding. How it reaches the MCP server depends on where the
// server came from:
//   - A server registered by URL is listed from the control plane, at that URL with the binding's stored upstream
//     credential, whatever the binding's gateway security. Through the gateway the answer would pass through the
//     binding's own policies (mcp-acl-list drops what it hides, mcp-rewrite renames), so the stored list would be
//     incomplete. A server the control plane cannot reach fails discovery; its capabilities are then sent with the
//     binding.
//   - A component's server (Integration Platform) has an in-cluster upstream of the data plane that AMS cannot reach,
//     so it is listed through the binding's public gateway URL. A binding secured with an API key is probed with a
//     short-lived console-managed key, issued for the attempt and revoked after it; one secured with a scheme AMS
//     cannot authenticate to fails.

const (
	mcpDiscoveryTickInterval = 10 * time.Second
	mcpDiscoveryBatch        = 20
	mcpDiscoveryBaseBackoff  = 10 * time.Second
	mcpDiscoveryMaxBackoff   = 5 * time.Minute
	// mcpDiscoveryAttemptTimeout bounds one row's attempt: the key's issue, its wait for the gateway, and the probe.
	mcpDiscoveryAttemptTimeout = 90 * time.Second
	// mcpDiscoveryBatchBudget leaves the claim lease a minute to record the last outcome.
	mcpDiscoveryBatchBudget = repositories.MCPCapabilityDiscoveryClaimLease - time.Minute
	// mcpDiscoveryAttemptBudget is about 30 minutes of backoff before the row is failed.
	mcpDiscoveryAttemptBudget = 10
	// mcpDiscoveryKeyWait is how long a probe keeps retrying a 401 while the gateway receives the attempt's key.
	mcpDiscoveryKeyWait = 30 * time.Second
	mcpDiscoveryKeyPoll = 2 * time.Second
	// mcpDiscoveryKeyName is the console-managed key a discovery attempt probes with. It is never listed, and its
	// prefix is one the binding key routes refuse, so no user key can share its name.
	mcpDiscoveryKeyName = models.APIKeyTestKeyPrefix + "capability-discovery"
	// mcpDiscoveryDefaultAPIKeyHeader is the header a binding's API key goes in when its security names none.
	mcpDiscoveryDefaultAPIKeyHeader = "X-API-Key"
)

// errMCPDiscoveryNotNeeded means the binding no longer wants discovery: it is gone, or its caller gave capabilities.
var errMCPDiscoveryNotNeeded = errors.New("capability discovery is not needed for this binding")

// MCPCapabilityProber lists an MCP server's capabilities at url, sending headerValue in headerName when set.
type MCPCapabilityProber func(ctx context.Context, url, headerName, headerValue string) (*models.MCPServerInfoFetchResponse, error)

// ProjectMCPCapabilityDiscoveryService drains the project MCP proxy capability discovery queue.
type ProjectMCPCapabilityDiscoveryService interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce drains the currently-due batch once.
	RunOnce(ctx context.Context)
}

type projectMCPCapabilityDiscoveryService struct {
	discoveries repositories.MCPCapabilityDiscoveryRepository
	bindings    *ProjectMCPProxyService
	probe       MCPCapabilityProber
	// probeUpstream probes an MCP server at its own URL, which its owner gave, so it is SSRF-guarded.
	probeUpstream MCPCapabilityProber
	logger        *slog.Logger
	stopCh        chan struct{}
	stopOnce      sync.Once

	attemptTimeout time.Duration
	batchBudget    time.Duration
	keyWait        time.Duration
	keyPoll        time.Duration
}

// NewProjectMCPCapabilityDiscoveryService creates the discovery reconciler.
func NewProjectMCPCapabilityDiscoveryService(
	discoveries repositories.MCPCapabilityDiscoveryRepository, bindings *ProjectMCPProxyService, logger *slog.Logger,
) ProjectMCPCapabilityDiscoveryService {
	client := newMCPDiscoveryClient(config.GetConfig().A2ACardFetchDialAddr)
	s := newProjectMCPCapabilityDiscoveryService(discoveries, bindings, newMCPDiscoveryProber(client, logger), logger)
	s.probeUpstream = newMCPDiscoveryProber(newMCPUpstreamDiscoveryClient(), logger)
	return s
}

// newMCPUpstreamDiscoveryClient is the client that probes an MCP server at its upstream URL.
func newMCPUpstreamDiscoveryClient() *http.Client {
	client := ssrf.NewClient(mcpRequestTimeout)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

// newMCPDiscoveryClient is the probe's HTTP client. It is not the SSRF-guarded one: the URL is built by AMS from a
// managed gateway's vhost, never given by a user, and resolves to loopback in local setups. What answers is tenant
// code behind the gateway, though, so it follows no redirect: one could point AMS at an internal address, carry the
// key there (Go forwards custom headers across hosts), and bring the answer back through lastError. Like the A2A
// platform fetch, it ignores proxy env and can pin its dial address.
func newMCPDiscoveryClient(dialAddr string) *http.Client {
	return &http.Client{
		Timeout:   mcpRequestTimeout,
		Transport: plainTransport(dialAddr),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// newMCPDiscoveryProber probes with the MCP server-info client, each probe with a logger that redacts its key: the
// client's helpers log what the server answered, and a server may echo the request's key back. A failed list fails
// the probe, so the attempt goes through backoff instead of storing that list as empty.
func newMCPDiscoveryProber(client *http.Client, logger *slog.Logger) MCPCapabilityProber {
	return func(ctx context.Context, url, headerName, headerValue string) (*models.MCPServerInfoFetchResponse, error) {
		prober := &MCPProxyService{client: client, logger: redactingLogger(logger, headerValue), strictLists: true}
		return prober.fetchMCPServerInfo(ctx, url, headerName, headerValue)
	}
}

func newProjectMCPCapabilityDiscoveryService(
	discoveries repositories.MCPCapabilityDiscoveryRepository, bindings *ProjectMCPProxyService,
	probe MCPCapabilityProber, logger *slog.Logger,
) *projectMCPCapabilityDiscoveryService {
	return &projectMCPCapabilityDiscoveryService{
		discoveries:   discoveries,
		bindings:      bindings,
		probe:         probe,
		probeUpstream: probe,
		logger:        logger,
		stopCh:        make(chan struct{}),

		attemptTimeout: mcpDiscoveryAttemptTimeout,
		batchBudget:    mcpDiscoveryBatchBudget,
		keyWait:        mcpDiscoveryKeyWait,
		keyPoll:        mcpDiscoveryKeyPoll,
	}
}

func (s *projectMCPCapabilityDiscoveryService) Start(ctx context.Context) error {
	go s.runLoop(ctx)
	s.logger.Info("MCP capability discovery started")
	return nil
}

func (s *projectMCPCapabilityDiscoveryService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("MCP capability discovery stopped")
	})
	return nil
}

func (s *projectMCPCapabilityDiscoveryService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(mcpDiscoveryTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.RunOnce(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *projectMCPCapabilityDiscoveryService) RunOnce(ctx context.Context) {
	claimedAt := time.Now()
	due, err := s.discoveries.ClaimDue(ctx, claimedAt, mcpDiscoveryBatch)
	if err != nil {
		s.logger.Error("Failed to claim due MCP capability discoveries", "error", err)
		return
	}
	drainWithinLease(ctx, s.logger, claimedAt.Add(s.batchBudget), due, "MCP capability discovery", s.discoverOne)
}

// discoverOne runs one row's attempt and records its outcome.
func (s *projectMCPCapabilityDiscoveryService) discoverOne(ctx context.Context, row models.MCPCapabilityDiscovery) {
	attemptCtx, recordCtx, cancel := a2aAttemptContexts(ctx, s.attemptTimeout)
	defer cancel()
	log := s.logger.With("ouID", row.OUID, "artifactUUID", row.ArtifactUUID)

	capabilities, err := s.attempt(attemptCtx, row)
	var permanent *mcpDiscoveryPermanentError
	switch {
	case errors.Is(err, errMCPDiscoveryNotNeeded):
		s.record(log, s.discoveries.Drop(recordCtx, row))
		return
	case errors.As(err, &permanent):
		log.Info("MCP capability discovery cannot succeed for this binding", "reason", permanent.Error())
		s.record(log, s.discoveries.MarkFailed(recordCtx, row, permanent.Error()))
		return
	case err != nil:
		s.recordAttemptFailure(recordCtx, log, row, err)
		return
	}
	err = s.bindings.storeDiscoveredCapabilities(recordCtx, row, capabilities)
	switch {
	case errors.Is(err, errMCPDiscoveryNotNeeded):
		s.record(log, s.discoveries.Drop(recordCtx, row))
	case err != nil && !errors.Is(err, repositories.ErrMCPCapabilityDiscoverySuperseded):
		s.recordAttemptFailure(recordCtx, log, row, fmt.Errorf("failed to store the discovered capabilities: %w", err))
	default:
		s.record(log, err)
		if err == nil {
			log.Info("Discovered the MCP server's capabilities", "tools", capabilityCount(capabilities))
		}
	}
}

// mcpDiscoveryPermanentError is a reason discovery cannot succeed until the binding changes; the row is failed
// without retrying.
type mcpDiscoveryPermanentError struct{ reason string }

func (e *mcpDiscoveryPermanentError) Error() string { return e.reason }

// attempt lists the binding's capabilities, at the MCP server's own URL or through the binding's gateway (see
// discoveryTarget).
func (s *projectMCPCapabilityDiscoveryService) attempt(
	ctx context.Context, row models.MCPCapabilityDiscovery,
) (*models.MCPProxyCapabilities, error) {
	target, err := s.bindings.discoveryTarget(ctx, row.OUID, row.ArtifactUUID)
	if err != nil {
		return nil, err
	}
	if target.upstream {
		info, err := s.probeUpstream(ctx, target.probeURL, target.headerName, target.headerValue)
		if err != nil {
			return nil, upstreamProbeError(err, target)
		}
		capabilities, err := normalizeDiscoveredCapabilities(info)
		if err != nil {
			return nil, err
		}
		return scrubCapabilities(capabilities, upstreamSecrets(target))
	}
	headerName, headerValue := "", ""
	if target.security.RequiresAPIKey() {
		name, in := target.security.APIKeyNameAndLocation(mcpDiscoveryDefaultAPIKeyHeader)
		// The key only ever goes in a header: the gateway's api-key policy checks no other location (see
		// SecurityConfig.ValidateAPIKeyLocation), and a key in the URL would surface in transport errors, which are
		// logged and shown as lastError.
		if in != "header" {
			return nil, &mcpDiscoveryPermanentError{reason: fmt.Sprintf(
				"the binding's API key is read from the %s, which the gateway does not check; use a header", in,
			)}
		}
		value, revoke, err := s.bindings.issueDiscoveryKey(ctx, row.OUID, target.artifact)
		if err != nil {
			return nil, fmt.Errorf("failed to issue the discovery key: %w", err)
		}
		defer revoke()
		headerName, headerValue = name, value
	}
	info, err := s.probeUntilKeyArrives(ctx, target.probeURL, headerName, headerValue)
	if err != nil {
		return nil, fmt.Errorf("failed to list the MCP server's capabilities at %s: %w",
			target.probeURL, redactSecret(err, headerValue))
	}
	return normalizeDiscoveredCapabilities(info)
}

// upstreamProbeError is a failed probe of an MCP server at its own URL, with the probe's secrets redacted. A URL the
// control plane refuses to call (not publicly routable) and a credential the server rejects (401) cannot get better
// until the binding changes, and a binding change queues discovery again, so both are permanent. Any other failure is
// retried; its reason says the control plane could not reach the server, so the last one recorded tells the user
// what to do. The endpoint it names is scrubbed too: a credential can sit in the URL's path.
func upstreamProbeError(err error, target *mcpDiscoveryTarget) error {
	endpoint := scrubString(safeEndpoint(target.probeURL), upstreamSecrets(target))
	switch {
	case errors.Is(err, utils.ErrInvalidURL):
		return &mcpDiscoveryPermanentError{reason: fmt.Sprintf(
			"the control plane does not call the MCP server at %s: its address is not publicly reachable; "+
				"send capabilities with the binding", endpoint)}
	case errors.Is(err, utils.ErrMCPServerUnauthorized):
		return &mcpDiscoveryPermanentError{reason: fmt.Sprintf(
			"the MCP server at %s rejected the binding's upstream credential (401); update the credential or "+
				"send capabilities with the binding", endpoint)}
	}
	return fmt.Errorf("the control plane could not list the MCP server's capabilities at %s (if only the gateway "+
		"can reach the server, send capabilities with the binding): %w", endpoint, redactUpstream(err, target))
}

// normalizeDiscoveredCapabilities is a probe's answer as a binding stores it; capabilities it cannot store are a
// permanent failure.
func normalizeDiscoveredCapabilities(info *models.MCPServerInfoFetchResponse) (*models.MCPProxyCapabilities, error) {
	capabilities, err := normalizeBindingCapabilities(&models.MCPProxyCapabilities{
		Tools: info.Tools, Prompts: info.Prompts, Resources: info.Resources,
	})
	if err != nil {
		return nil, &mcpDiscoveryPermanentError{reason: fmt.Sprintf("the MCP server's capabilities cannot be stored: %v", err)}
	}
	return capabilities, nil
}

// probeUntilKeyArrives probes, retrying a 401 for keyWait: a key issued for the attempt reaches the gateway through
// the event hub, a moment after it is stored.
func (s *projectMCPCapabilityDiscoveryService) probeUntilKeyArrives(
	ctx context.Context, probeURL, headerName, headerValue string,
) (*models.MCPServerInfoFetchResponse, error) {
	deadline := time.Now().Add(s.keyWait)
	for {
		info, err := s.probe(ctx, probeURL, headerName, headerValue)
		if err == nil || headerValue == "" || !errors.Is(err, utils.ErrMCPServerUnauthorized) {
			return info, err
		}
		if time.Now().Add(s.keyPoll).After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(s.keyPoll):
		}
	}
}

func (s *projectMCPCapabilityDiscoveryService) recordAttemptFailure(
	ctx context.Context, log *slog.Logger, row models.MCPCapabilityDiscovery, cause error,
) {
	if row.AttemptCount+1 >= mcpDiscoveryAttemptBudget {
		log.Warn("MCP capabilities could not be discovered within the attempt budget", "error", cause)
		s.record(log, s.discoveries.MarkFailed(ctx, row, cause.Error()))
		return
	}
	log.Debug("MCP capabilities not discoverable yet, will retry", "attempt", row.AttemptCount+1, "reason", cause)
	next := time.Now().Add(utils.ExponentialBackoff(row.AttemptCount, mcpDiscoveryBaseBackoff, mcpDiscoveryMaxBackoff))
	s.record(log, s.discoveries.MarkAttemptFailed(ctx, row, cause.Error(), next))
}

// record logs a failure to record an outcome. A row queued again meanwhile is left for its next claim.
func (s *projectMCPCapabilityDiscoveryService) record(log *slog.Logger, err error) {
	switch {
	case errors.Is(err, repositories.ErrMCPCapabilityDiscoverySuperseded):
		log.Info("MCP capability discovery was queued again during the attempt; leaving it for the next claim")
	case err != nil:
		log.Error("Failed to record the MCP capability discovery outcome", "error", err)
	}
}

func capabilityCount(c *models.MCPProxyCapabilities) int {
	if c == nil || c.Tools == nil {
		return 0
	}
	return len(*c.Tools)
}

// redactingLogger is logger with secret replaced in every message and attribute it writes; logger itself when secret
// is empty.
func redactingLogger(logger *slog.Logger, secret string) *slog.Logger {
	if logger == nil || secret == "" {
		return logger
	}
	return slog.New(&redactingHandler{next: logger.Handler(), secret: secret})
}

// redactingHandler is a slog.Handler that hides a secret from what it passes on.
type redactingHandler struct {
	next   slog.Handler
	secret string
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, h.redact(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = h.redactAttr(a)
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted), secret: h.secret}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name), secret: h.secret}
}

func (h *redactingHandler) redact(text string) string {
	return strings.ReplaceAll(text, h.secret, "[REDACTED]")
}

// redactAttr redacts a string, an error's or a stringer's text, and a group's members; other values pass as they are.
func (h *redactingHandler) redactAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, h.redact(v.String()))
	case slog.KindGroup:
		members := v.Group()
		redacted := make([]any, len(members))
		for i, m := range members {
			redacted[i] = h.redactAttr(m)
		}
		return slog.Group(a.Key, redacted...)
	case slog.KindAny:
		if text := fmt.Sprint(v.Any()); strings.Contains(text, h.secret) {
			return slog.String(a.Key, h.redact(text))
		}
	}
	return slog.Attr{Key: a.Key, Value: v}
}

// redactedError is an error whose text has a secret replaced; errors.Is and errors.As still see the original.
type redactedError struct {
	err  error
	text string
}

func (e *redactedError) Error() string { return e.text }
func (e *redactedError) Unwrap() error { return e.err }

// redactSecret hides secret in err's text, so an error that echoes a credential (a server repeating a header back,
// say) is never logged or stored as lastError with it.
func redactSecret(err error, secret string) error {
	if err == nil || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	return &redactedError{err: err, text: strings.ReplaceAll(err.Error(), secret, "[REDACTED]")}
}

// mcpDiscoveryMinQuerySecret is the shortest query or userinfo value redacted on its own. A shorter one (?format=json)
// is too likely to be an ordinary word to blank out of every description; the whole query, which contains it, is
// still redacted.
const mcpDiscoveryMinQuerySecret = 6

// upstreamSecrets is what an upstream probe must never let reach lastError, the logs or the stored capabilities: the
// credential header's value, its credential without a scheme ("Bearer <token>" also hides "<token>"), and what the
// registered URL carries besides its endpoint: its query, each query value (decoded and as encoded), and its
// userinfo and password. Longest first, so a secret that contains another is replaced whole.
func upstreamSecrets(target *mcpDiscoveryTarget) []string {
	seen := map[string]bool{}
	var secrets []string
	add := func(secret string, minLen int) {
		secret = strings.TrimSpace(secret)
		if len(secret) < minLen || seen[secret] {
			return
		}
		seen[secret] = true
		secrets = append(secrets, secret)
	}
	if v := strings.TrimSpace(target.headerValue); v != "" {
		add(v, 1)
		if _, credential, ok := strings.Cut(v, " "); ok {
			add(credential, 1)
		}
	}
	if u, err := url.Parse(target.probeURL); err == nil {
		add(u.RawQuery, 1)
		for _, values := range u.Query() {
			for _, value := range values {
				add(value, mcpDiscoveryMinQuerySecret)
				add(url.QueryEscape(value), mcpDiscoveryMinQuerySecret)
			}
		}
		if u.User != nil {
			add(u.User.String(), 1)
			if password, ok := u.User.Password(); ok {
				add(password, mcpDiscoveryMinQuerySecret)
			}
		}
	}
	sort.SliceStable(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

// redactUpstream is an upstream probe's error with each of upstreamSecrets hidden; a transport error that names the
// whole URL names it without its query or userinfo.
func redactUpstream(err error, target *mcpDiscoveryTarget) error {
	for _, secret := range upstreamSecrets(target) {
		err = redactSecret(err, secret)
	}
	return err
}

// safeEndpoint is a registered URL as a diagnostic names it: scheme, host and path, with no userinfo, query or
// fragment, which may carry credentials.
func safeEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "the registered URL"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// scrubCapabilities hides secrets in every string an MCP server returned in its capabilities (a description that
// echoes the request's credential, say) and in every map key, so a binding read never returns one. It walks the
// values, so numbers, booleans and nulls are kept as they are and the schemas stay valid.
func scrubCapabilities(capabilities *models.MCPProxyCapabilities, secrets []string) (*models.MCPProxyCapabilities, error) {
	if capabilities == nil || len(secrets) == 0 {
		return capabilities, nil
	}
	tools, err := scrubCapabilityList(capabilities.Tools, secrets)
	if err != nil {
		return nil, err
	}
	prompts, err := scrubCapabilityList(capabilities.Prompts, secrets)
	if err != nil {
		return nil, err
	}
	resources, err := scrubCapabilityList(capabilities.Resources, secrets)
	if err != nil {
		return nil, err
	}
	return &models.MCPProxyCapabilities{Tools: tools, Prompts: prompts, Resources: resources}, nil
}

func scrubCapabilityList(list *[]map[string]interface{}, secrets []string) (*[]map[string]interface{}, error) {
	if list == nil {
		return nil, nil //nolint:nilnil // an absent list stays absent
	}
	out := make([]map[string]interface{}, len(*list))
	for i, entry := range *list {
		scrubbed, err := scrubValue(entry, secrets)
		if err != nil {
			return nil, err
		}
		out[i], _ = scrubbed.(map[string]interface{})
	}
	return &out, nil
}

// scrubString is text with every secret replaced.
func scrubString(text string, secrets []string) string {
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	return text
}

// scrubValue is v with secrets replaced in every string it holds, map keys included; the containers are copied, never
// changed in place. A key is scrubbed like a value, so a schema's references to it (such as a required list, whose
// entries are strings) are scrubbed the same way and still match. Two keys of one map that scrub to the same key would
// lose one of them, so that document is refused instead.
func scrubValue(v interface{}, secrets []string) (interface{}, error) {
	switch t := v.(type) {
	case string:
		return scrubString(t, secrets), nil
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, item := range t {
			key := scrubString(k, secrets)
			if _, taken := out[key]; taken {
				return nil, &mcpDiscoveryPermanentError{
					reason: "the MCP server's capabilities echo the binding's upstream credential in their field names " +
						"and cannot be stored; send capabilities with the binding",
				}
			}
			scrubbed, err := scrubValue(item, secrets)
			if err != nil {
				return nil, err
			}
			out[key] = scrubbed
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			scrubbed, err := scrubValue(item, secrets)
			if err != nil {
				return nil, err
			}
			out[i] = scrubbed
		}
		return out, nil
	case []map[string]interface{}:
		out := make([]map[string]interface{}, len(t))
		for i, item := range t {
			scrubbed, err := scrubValue(item, secrets)
			if err != nil {
				return nil, err
			}
			out[i], _ = scrubbed.(map[string]interface{})
		}
		return out, nil
	case []string:
		out := make([]string, len(t))
		for i, item := range t {
			out[i] = scrubString(item, secrets)
		}
		return out, nil
	default:
		return v, nil
	}
}

// mcpDiscoveryTarget is what a discovery attempt needs of a binding.
type mcpDiscoveryTarget struct {
	artifact string
	// probeURL is the URL discovery lists the MCP server at: the binding's public gateway URL, or the MCP server's own
	// URL when upstream is set.
	probeURL string
	security *models.SecurityConfig
	// upstream marks a target probed at the MCP server's own URL instead of through the gateway, with the binding's
	// upstream credential in headerName/headerValue when it has one.
	upstream    bool
	headerName  string
	headerValue string
}

// bindingByArtifact finds the project proxy binding whose gateway artifact is artifactUUID. errMCPDiscoveryNotNeeded
// when there is none, or it belongs to an org proxy.
func (s *ProjectMCPProxyService) bindingByArtifact(
	ctx context.Context, ouID string, artifactUUID uuid.UUID,
) (*models.MCPProxy, *models.MCPProxyEndpoint, *models.MCPProxyEndpointEnvironment, error) {
	ee, err := s.base.endpointRepo.GetEndpointEnvByArtifact(ctx, artifactUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, errMCPDiscoveryNotNeeded
	}
	if err != nil {
		return nil, nil, nil, err
	}
	proxy, err := s.base.repo.GetByUUID(ctx, ee.MCPProxyUUID.String(), ouID)
	if errors.Is(err, utils.ErrMCPProxyNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, errMCPDiscoveryNotNeeded
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read the MCP proxy: %w", err)
	}
	if proxy.ProjectUUID == nil {
		return nil, nil, nil, errMCPDiscoveryNotNeeded
	}
	for i := range proxy.Endpoints {
		if proxy.Endpoints[i].UUID == ee.EndpointUUID {
			return proxy, &proxy.Endpoints[i], ee, nil
		}
	}
	return nil, nil, nil, errMCPDiscoveryNotNeeded
}

// callerGaveCapabilities reports a binding whose capabilities came from its caller, which discovery leaves alone.
func callerGaveCapabilities(cfg models.MCPEndpointConfig) bool {
	return cfg.Capabilities != nil && !cfg.CapabilitiesDiscovered
}

// discoveryTarget is where a deployed binding's MCP server is listed and with what credential. A binding not deployed
// yet is an attempt failure, retried. A server registered by URL is listed at that URL from the control plane, with
// the binding's upstream credential (directDiscoveryTarget), whatever the binding's security. A component's server is
// listed through the binding's public gateway URL: with a discovery key when the binding requires an API key, with
// none when its security is off or absent. A component binding secured with any other scheme (such as Agent
// Identity) cannot be probed through the gateway and is a permanent failure.
func (s *ProjectMCPProxyService) discoveryTarget(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*mcpDiscoveryTarget, error) {
	proxy, endpoint, ee, err := s.bindingByArtifact(ctx, ouID, artifactUUID)
	if err != nil {
		return nil, err
	}
	if callerGaveCapabilities(endpoint.Configuration) {
		return nil, errMCPDiscoveryNotNeeded
	}
	resp, err := s.environmentResponse(ctx, ouID, proxy, endpoint, ee)
	if err != nil {
		return nil, err
	}
	// Discovery runs for a deployed binding only, however it is reached: a refresh queued while the binding deploys
	// waits for it.
	if resp.DeploymentStatus != models.MCPDeploymentStatusDeployed {
		status := resp.DeploymentStatus
		if status == "" {
			status = resp.DeploymentStatusReason
		}
		return nil, fmt.Errorf("the binding is not deployed (%s)", status)
	}
	if direct, err := s.directDiscoveryTarget(proxy, endpoint, ee); direct != nil || err != nil {
		return direct, err
	}
	security := endpoint.Configuration.Security
	if security != nil && (security.Enabled == nil || *security.Enabled) && !security.RequiresAPIKey() {
		return nil, &mcpDiscoveryPermanentError{
			reason: "the binding is secured with a scheme AMS cannot authenticate to, and its MCP server is " +
				"reachable only through the gateway; send capabilities with the binding",
		}
	}
	if resp.PublicURL == "" {
		return nil, fmt.Errorf("the binding's gateway has no public URL")
	}
	return &mcpDiscoveryTarget{artifact: ee.ArtifactUUID.String(), probeURL: resp.PublicURL, security: security}, nil
}

// directDiscoveryTarget is the target of a binding whose MCP server was registered by URL: Agent Manager lists it from
// the control plane, at that URL with the binding's upstream credential, as the org proxy flow does, whatever the
// binding's gateway security. Through the gateway the answer would have been through the binding's policies
// (mcp-acl-list drops what it hides, mcp-rewrite renames and filters), so its stored list would lose capabilities.
// nil for a component's MCP server, whose upstream is an in-cluster address of the data plane that only the gateway
// reaches. An MCP server the control plane cannot reach fails discovery; its capabilities are then given with the
// binding.
func (s *ProjectMCPProxyService) directDiscoveryTarget(
	proxy *models.MCPProxy, endpoint *models.MCPProxyEndpoint, ee *models.MCPProxyEndpointEnvironment,
) (*mcpDiscoveryTarget, error) {
	upstream := endpoint.Configuration.Upstream
	if proxy.Configuration.Source != nil || upstream == nil || strings.TrimSpace(upstream.URL) == "" {
		return nil, nil //nolint:nilnil // not a direct target
	}
	target := &mcpDiscoveryTarget{artifact: ee.ArtifactUUID.String(), probeURL: strings.TrimSpace(upstream.URL), upstream: true}
	if auth := upstream.Auth; auth != nil && strings.EqualFold(valueOrEmpty(auth.Type), "api-key") {
		value, err := s.storedUpstreamValue(auth)
		if err != nil {
			return nil, &mcpDiscoveryPermanentError{reason: "the binding's upstream credential cannot be read"}
		}
		target.headerName, target.headerValue = strings.TrimSpace(valueOrEmpty(auth.Header)), value
	}
	return target, nil
}

// issueDiscoveryKey issues the console-managed key a discovery attempt probes with, sent to the organization's
// gateways like the binding's other keys. revoke removes it; it is called once the attempt is done, whatever its
// outcome, with a context of its own so a timed-out attempt still revokes.
func (s *ProjectMCPProxyService) issueDiscoveryKey(ctx context.Context, ouID, artifact string) (string, func(), error) {
	gateways, err := s.organizationGateways(ctx, ouID)
	if err != nil {
		return "", nil, err
	}
	expiresAt := time.Now().UTC().Add(mcpDiscoveryAttemptTimeout + time.Minute).Format(time.RFC3339)
	resp, err := s.base.apiKeyBroadcaster.broadcastCreateToGateways(gateways, ouID, artifact, artifact, &models.CreateAPIKeyRequest{
		Name:        mcpDiscoveryKeyName,
		DisplayName: "Capability discovery",
		Purpose:     models.APIKeyPurposeConsoleManaged,
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		return "", nil, err
	}
	revoke := func() {
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := s.base.apiKeyBroadcaster.broadcastRevokeToGateways(revokeCtx, gateways, artifact, artifact, mcpDiscoveryKeyName); err != nil {
			s.base.logger.Warn("Failed to revoke the MCP capability discovery key; it expires on its own",
				"ouID", ouID, "artifactUUID", artifact, "error", err)
		}
	}
	return resp.APIKey, revoke, nil
}

// storeDiscoveredCapabilities writes discovered capabilities onto the binding and marks its discovery discovered. The
// proxy row is locked, so a binding PUT cannot interleave, and the write is one statement conditional on the claim's
// token (StoreDiscovered), so a discovery queued again since the claim, which does not take the proxy lock, makes it
// a no-op (ErrMCPCapabilityDiscoverySuperseded) instead of being overwritten. A binding whose caller has since given
// capabilities, or that is gone, is left alone (errMCPDiscoveryNotNeeded). The binding is not redeployed: the
// gateway does not carry capabilities.
func (s *ProjectMCPProxyService) storeDiscoveredCapabilities(
	ctx context.Context, row models.MCPCapabilityDiscovery, capabilities *models.MCPProxyCapabilities,
) error {
	proxy, _, _, err := s.bindingByArtifact(ctx, row.OUID, row.ArtifactUUID)
	if err != nil {
		return err
	}
	return s.base.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.base.repo.GetByHandleForUpdate(ctx, tx, proxyHandleOf(proxy), row.OUID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errMCPDiscoveryNotNeeded
		}
		if err != nil {
			return fmt.Errorf("failed to lock the MCP proxy: %w", err)
		}
		var endpoint *models.MCPProxyEndpoint
		for i := range locked.Endpoints {
			for _, env := range locked.Endpoints[i].Environments {
				if env.ArtifactUUID == row.ArtifactUUID {
					endpoint = &locked.Endpoints[i]
				}
			}
		}
		if endpoint == nil || callerGaveCapabilities(endpoint.Configuration) {
			return errMCPDiscoveryNotNeeded
		}
		return s.discoveries.WithTx(tx).StoreDiscovered(ctx, row, capabilities, time.Now())
	})
}
