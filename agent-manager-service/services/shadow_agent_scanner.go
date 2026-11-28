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
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Scan worker settings.
const (
	shadowScanTick        = 5 * time.Second
	shadowScanLease       = 5 * time.Minute
	shadowScanRenewEvery  = time.Minute
	shadowScanBatch       = 2
	shadowScanMaxAttempts = 3
	// shadowConnectionTimeout bounds one connection's scan.
	shadowConnectionTimeout = 30 * time.Minute
	shadowConnectionWorkers = 4
	// shadowScanProgressEvery bounds how often a running scan writes the
	// calls its connections made so far; a connection starting or ending is
	// written at once.
	shadowScanProgressEvery = 2 * time.Second
	// shadowScanWatchEvery is how often a running scan checks it was not
	// cancelled.
	shadowScanWatchEvery = 3 * time.Second
)

// errShadowScanCancelled is the cause of a scan's context when the user
// cancelled the scan.
var errShadowScanCancelled = errors.New("the scan was cancelled")

// Connection outcomes in a scan result.
const (
	shadowConnectionOK      = "ok"
	shadowConnectionPartial = "partial"
	shadowConnectionFailed  = "failed"
)

// ShadowAgentScanner runs queued Shadow Agent Discovery scans in the
// background. Scans are leased in the database, so several service replicas
// can run it; a scan whose worker died is picked up again when its lease
// runs out.
type ShadowAgentScanner interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce claims due scans and runs them.
	RunOnce(ctx context.Context)
}

type shadowAgentScanner struct {
	repo          repositories.ShadowAgentRepository
	connectors    ShadowConnectors
	encryptionKey []byte
	logger        *slog.Logger
	now           func() time.Time
	progressEvery time.Duration
	watchEvery    time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewShadowAgentScanner creates the scan worker.
func NewShadowAgentScanner(
	repo repositories.ShadowAgentRepository, connectors ShadowConnectors, encryptionKey []byte, logger *slog.Logger,
) ShadowAgentScanner {
	return &shadowAgentScanner{
		repo: repo, connectors: connectors, encryptionKey: encryptionKey,
		logger: logger.With("component", "shadow-agent-scanner"), now: time.Now,
		progressEvery: shadowScanProgressEvery, watchEvery: shadowScanWatchEvery,
	}
}

func (s *shadowAgentScanner) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("shadow agent scanner already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(shadowScanTick)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				s.RunOnce(runCtx)
			}
		}
	}()
	s.logger.Info("Shadow agent scanner started")
	return nil
}

func (s *shadowAgentScanner) Stop() error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel = nil
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	return nil
}

func (s *shadowAgentScanner) RunOnce(ctx context.Context) {
	now := s.now().UTC()
	// Pending source code scans are never claimed or expired: each waits for
	// its results until they are published or uploaded, or it is cancelled.
	scans, err := s.repo.ClaimDueScans(ctx, now, shadowScanLease, shadowScanBatch)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Error("Failed to claim shadow agent scans", "error", err)
		}
		return
	}
	for i := range scans {
		s.runScan(ctx, &scans[i])
	}
}

// runScan runs every connection of a scan, keeps its lease alive, writes
// its progress, stops when the user cancels it, and records the outcome.
func (s *shadowAgentScanner) runScan(ctx context.Context, scan *models.ShadowScan) {
	log := s.logger.With("orgId", scan.OUID, "scanId", scan.ID)
	// Record the outcome even when the worker is shutting down.
	record := context.WithoutCancel(ctx)
	if scan.Attempts > shadowScanMaxAttempts {
		s.giveUp(record, scan, log)
		return
	}
	log.Info("Shadow agent scan started", "connections", len(scan.ConnectionIDs))

	scanCtx, cancelScan := context.WithCancelCause(ctx)
	defer cancelScan(nil)
	progress := newShadowScanProgress(scan, s.repo, cancelScan, log)
	progress.flush(record)
	watchCtx, stopWatch := context.WithCancel(scanCtx)
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		s.watchScan(watchCtx, scan.ID, progress, cancelScan, log)
	}()

	startedAt := s.now().UTC()
	if scan.StartedAt != nil {
		startedAt = *scan.StartedAt
	}

	results := make([]models.ShadowScanConnectionResult, len(scan.ConnectionIDs))
	sem := make(chan struct{}, shadowConnectionWorkers)
	var wg sync.WaitGroup
	for i, idStr := range scan.ConnectionIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, idStr string) {
			defer wg.Done()
			defer func() { <-sem }()
			res := s.scanConnection(scanCtx, record, scan, i, idStr, startedAt, progress, log)
			if errors.Is(context.Cause(scanCtx), errShadowScanCancelled) {
				return // the cancel recorded the scan's results
			}
			if res.FinishedAt == nil {
				finished := s.now().UTC()
				res.FinishedAt = &finished
			}
			results[i] = res
			progress.finish(record, i, res)
		}(i, idStr)
	}
	wg.Wait()
	stopWatch()
	<-watched

	if errors.Is(context.Cause(scanCtx), errShadowScanCancelled) {
		// The cancel recorded the scan's results; agents of the connections
		// that finished before it are kept.
		if err := s.correlate(record, scan.OUID); err != nil {
			log.Error("Failed to correlate agents across connections", "error", err)
		}
		log.Info("Shadow agent scan cancelled")
		return
	}

	total := 0
	okCount, failCount := 0, 0
	for _, r := range results {
		total += r.FindingsCount
		switch r.Outcome {
		case shadowConnectionOK, models.ShadowCallSkipped:
			okCount++
		case shadowConnectionPartial:
		default:
			failCount++
		}
	}
	if err := s.correlate(record, scan.OUID); err != nil {
		log.Error("Failed to correlate agents across connections", "error", err)
	}
	finished := s.now().UTC()
	scan.FinishedAt = &finished
	scan.Results = results
	scan.FindingsCount = total
	switch {
	case okCount == len(results):
		scan.Status = models.ShadowScanSucceeded
	case failCount == len(results):
		scan.Status = models.ShadowScanFailed
	default:
		scan.Status = models.ShadowScanPartial
	}
	ok, err := s.repo.FinishScan(record, scan)
	if err != nil {
		log.Error("Failed to record the scan outcome", "error", err)
		return
	}
	if !ok {
		log.Info("Shadow agent scan was cancelled before it finished; its outcome is not recorded")
		return
	}
	log.Info("Shadow agent scan finished", "status", scan.Status, "agents", total)
}

// giveUp fails a scan whose workers stopped too many times: each connection
// the last attempt did not finish fails.
func (s *shadowAgentScanner) giveUp(ctx context.Context, scan *models.ShadowScan, log *slog.Logger) {
	msg := fmt.Sprintf("gave up after %d attempts", shadowScanMaxAttempts)
	scan.Status = models.ShadowScanFailed
	finished := s.now().UTC()
	scan.FinishedAt = &finished
	conns, err := s.repo.ListConnections(ctx, scan.OUID)
	if err != nil {
		log.Warn("Failed to list the connections of an abandoned scan", "error", err)
	}
	results, _ := repositories.FailUnfinishedScanResults(scan, conns, msg)
	if len(results) == 0 {
		results = append(results, models.ShadowScanConnectionResult{
			Outcome: shadowConnectionFailed, Message: msg, Calls: []models.ShadowScanCallResult{},
		})
	}
	scan.Results = results
	// Connections that finished saved their agents.
	scan.FindingsCount = repositories.ShadowResultsFindingsCount(results)
	if _, err := s.repo.FinishScan(ctx, scan); err != nil {
		log.Error("Failed to record an abandoned scan", "error", err)
	}
}

// watchScan runs while a scan runs: it writes the scan's progress, renews
// its lease, and cancels the scan when it is no longer running (the user
// cancelled it).
func (s *shadowAgentScanner) watchScan(
	ctx context.Context, id uuid.UUID, progress *shadowScanProgress, cancelScan context.CancelCauseFunc, log *slog.Logger,
) {
	progressTick := time.NewTicker(s.progressEvery)
	defer progressTick.Stop()
	watchTick := time.NewTicker(s.watchEvery)
	defer watchTick.Stop()
	leaseTick := time.NewTicker(shadowScanRenewEvery)
	defer leaseTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-progressTick.C:
			progress.flush(ctx)
		case <-watchTick.C:
			running, err := s.repo.ScanRunning(ctx, id)
			switch {
			case err != nil:
				if ctx.Err() == nil {
					log.Warn("Failed to check whether the scan was cancelled", "error", err)
				}
			case !running:
				cancelScan(errShadowScanCancelled)
				return
			}
		case <-leaseTick.C:
			if err := s.repo.RenewScanLease(ctx, id, s.now().UTC().Add(shadowScanLease)); err != nil && ctx.Err() == nil {
				log.Warn("Failed to renew the scan lease", "error", err)
			}
		}
	}
}

// shadowScanProgress holds a running scan's results so far, one per
// connection, and writes them to the scan. Connections run concurrently, so
// every change takes mu; writes take writeMu, so a write never replaces a
// newer one.
type shadowScanProgress struct {
	scanID     uuid.UUID
	repo       repositories.ShadowAgentRepository
	cancelScan context.CancelCauseFunc
	log        *slog.Logger

	writeMu sync.Mutex
	mu      sync.Mutex
	results []models.ShadowScanConnectionResult
	dirty   bool
}

func newShadowScanProgress(
	scan *models.ShadowScan, repo repositories.ShadowAgentRepository, cancelScan context.CancelCauseFunc, log *slog.Logger,
) *shadowScanProgress {
	p := &shadowScanProgress{
		scanID: scan.ID, repo: repo, cancelScan: cancelScan, log: log,
		results: make([]models.ShadowScanConnectionResult, len(scan.ConnectionIDs)), dirty: true,
	}
	for i, id := range scan.ConnectionIDs {
		p.results[i] = models.ShadowScanConnectionResult{
			ConnectionID: id, Outcome: models.ShadowConnectionQueued, Calls: []models.ShadowScanCallResult{},
		}
	}
	return p
}

// start marks connection i running and writes the progress.
func (p *shadowScanProgress) start(ctx context.Context, i int, name, provider string, at time.Time) {
	p.mu.Lock()
	r := &p.results[i]
	r.ConnectionName, r.Provider, r.Outcome, r.StartedAt = name, provider, models.ShadowConnectionRunning, &at
	p.dirty = true
	p.mu.Unlock()
	p.flush(ctx)
}

// calls records the calls connection i made so far; the next tick writes
// them.
func (p *shadowScanProgress) calls(i int, calls []models.ShadowScanCallResult) {
	calls = sanitizeCalls(calls)
	p.mu.Lock()
	p.results[i].Calls = calls
	p.dirty = true
	p.mu.Unlock()
}

// applyFinal runs apply, which saves connection i's agents and its final
// result in one transaction, and on success keeps final (with the agents
// written) as i's result. It holds the write lock throughout, so no progress
// snapshot taken before the apply is written after it.
func (p *shadowScanProgress) applyFinal(
	i int, final models.ShadowScanConnectionResult, apply func() (int, error),
) (int, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	n, err := apply()
	if err != nil {
		return n, err
	}
	final.FindingsCount = n
	p.mu.Lock()
	p.results[i] = final
	p.mu.Unlock()
	return n, nil
}

// finish records connection i's result and writes the progress.
func (p *shadowScanProgress) finish(ctx context.Context, i int, res models.ShadowScanConnectionResult) {
	p.mu.Lock()
	p.results[i] = res
	p.dirty = true
	p.mu.Unlock()
	p.flush(ctx)
}

// flush writes the progress when it changed. A failed write is retried by
// the next one and never fails the scan; a scan that is no longer running
// was cancelled, so its context is cancelled.
func (p *shadowScanProgress) flush(ctx context.Context) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.mu.Lock()
	if !p.dirty {
		p.mu.Unlock()
		return
	}
	snapshot := append([]models.ShadowScanConnectionResult(nil), p.results...)
	p.dirty = false
	p.mu.Unlock()
	running, err := p.repo.UpdateScanProgress(ctx, p.scanID, snapshot)
	if err != nil {
		p.mu.Lock()
		p.dirty = true
		p.mu.Unlock()
		if ctx.Err() == nil {
			p.log.Warn("Failed to record the scan progress", "error", err)
		}
		return
	}
	if !running {
		p.cancelScan(errShadowScanCancelled)
	}
}

// scanConnection runs one connection and writes what it found. A failure
// here fails only this connection's part of the scan.
//
// ctx is the scan's context: when the user cancels the scan, its cause is
// errShadowScanCancelled and nothing the connection found is saved.
func (s *shadowAgentScanner) scanConnection(
	ctx, record context.Context, scan *models.ShadowScan, i int, idStr string, startedAt time.Time,
	progress *shadowScanProgress, log *slog.Logger,
) models.ShadowScanConnectionResult {
	connStarted := s.now().UTC()
	res := models.ShadowScanConnectionResult{
		ConnectionID: idStr, Outcome: shadowConnectionFailed, Calls: []models.ShadowScanCallResult{}, StartedAt: &connStarted,
	}
	if errors.Is(context.Cause(ctx), errShadowScanCancelled) {
		res.Message = models.ShadowConnectionCancelledMessage
		return res
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		res.Message = "invalid connection ID"
		return res
	}
	c, err := s.repo.GetConnection(ctx, scan.OUID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res.Message = "the connection was deleted"
		} else {
			res.Message = "failed to load the connection"
			log.Error("Failed to load a connection", "connectionId", id, "error", err)
		}
		return res
	}
	res.ConnectionName, res.Provider = c.Name, c.Provider
	if spec, ok := sa.Provider(c.Provider); ok && !spec.CloudScanned() {
		// A scan queued before the platform stopped being scanned (Bring
		// your own inventory now takes uploaded files): nothing to read.
		res.Outcome, res.Message = models.ShadowCallSkipped, "not scanned: "+shadowNotScannedReason(spec)
		return res
	}
	connector, ok := s.connectors[c.Provider]
	if !ok {
		res.Message = "no connector for this platform"
		return res
	}
	conn, err := shadowRuntimeConnection(c, s.encryptionKey)
	if err != nil {
		res.Message = "the stored credential cannot be decrypted; enter it again"
		log.Error("Failed to decrypt a connection credential", "connectionId", id, "error", err)
		return res
	}
	progress.start(record, i, c.Name, c.Provider, connStarted)
	cctx, cancel := context.WithTimeout(ctx, shadowConnectionTimeout)
	defer cancel()
	cctx = sa.WithCallObserver(cctx, func(calls []models.ShadowScanCallResult) { progress.calls(i, calls) })
	out := runConnector(cctx, connector, conn, log)
	res.Calls = sanitizeCalls(out.Calls)
	if errors.Is(context.Cause(ctx), errShadowScanCancelled) {
		// Cancelled while it ran: what it found is not saved.
		res.Message = models.ShadowConnectionCancelledMessage
		return res
	}
	res.Outcome, res.Message = connectionOutcome(res.Calls)
	// A listing is complete only when every call succeeded (or was skipped
	// on purpose) and the scan was not cut short.
	complete := res.Outcome == shadowConnectionOK && cctx.Err() == nil
	finished := s.now().UTC()
	res.FinishedAt = &finished
	in := toShadowApply(scan, c, out, startedAt, finished, complete, log)
	// The agents and the connection's final result are saved together, and
	// only while the scan is still running.
	final := res
	in.Result, in.ResultIndex = &final, i
	n, err := progress.applyFinal(i, final, func() (int, error) { return s.repo.ApplyConnectionResult(record, in) })
	switch {
	case errors.Is(err, utils.ErrShadowScanNotRunning):
		// The user cancelled the scan before its agents were saved.
		progress.cancelScan(errShadowScanCancelled)
		res.Message, res.FinishedAt = models.ShadowConnectionCancelledMessage, nil
		res.Outcome = shadowConnectionFailed
		return res
	case err != nil:
		res.Outcome = shadowConnectionFailed
		res.Message = "failed to save what the scan found"
		log.Error("Failed to save a connection's scan result", "connectionId", id, "error", err)
		return res
	}
	res.FindingsCount = n
	return res
}

// shadowNotScannedReason says where the agents of a platform a cloud scan
// does not read come from.
func shadowNotScannedReason(spec sa.ProviderSpec) string {
	if spec.FileUpload {
		return "its agents come from uploaded inventory files"
	}
	return "its repositories are scanned by the scan command"
}

// runConnector runs a connector, turning a panic into a failed result so one
// provider cannot take the worker down.
func runConnector(ctx context.Context, c sa.Connector, conn sa.Connection, log *slog.Logger) (out *sa.Result) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("Shadow agent connector panicked", "provider", conn.Provider, "connectionId", conn.ID)
			out = &sa.Result{Calls: []models.ShadowScanCallResult{{Call: "scan", Outcome: models.ShadowCallFailed, Message: "internal error in the connector"}}}
		}
	}()
	out = c.Scan(ctx, conn)
	if out == nil {
		out = &sa.Result{}
	}
	return out
}

func sanitizeCalls(calls []models.ShadowScanCallResult) []models.ShadowScanCallResult {
	out := make([]models.ShadowScanCallResult, 0, len(calls))
	for _, c := range calls {
		c.Call = sa.Truncate(c.Call, 200)
		c.Message = sa.ScrubText(c.Message)
		out = append(out, c)
	}
	return out
}

// connectionOutcome summarizes a connection's calls: skipped when every call
// was skipped on purpose (nothing read), ok when every call
// succeeded or was skipped on purpose, auth_failed when the credential was
// refused and nothing was read, failed when nothing succeeded, partial
// otherwise.
func connectionOutcome(calls []models.ShadowScanCallResult) (string, string) {
	okCalls, failed, authFailed, skipped := 0, 0, 0, 0
	var firstErr, firstSkip string
	for _, c := range calls {
		switch c.Outcome {
		case models.ShadowCallOK:
			okCalls++
		case models.ShadowCallSkipped:
			skipped++
			if firstSkip == "" {
				firstSkip = c.Call + ": " + c.Message
			}
		default:
			failed++
			if c.Outcome == models.ShadowCallAuthFailed {
				authFailed++
			}
			if firstErr == "" {
				firstErr = c.Call + ": " + c.Message
			}
		}
	}
	switch {
	case failed == 0 && okCalls == 0 && skipped > 0:
		// Nothing was read on purpose (a source host until its analysis
		// exists): skipped, which does not fail the scan.
		return models.ShadowCallSkipped, sa.ScrubText(firstSkip)
	case failed == 0:
		return shadowConnectionOK, ""
	case okCalls == 0 && authFailed > 0:
		return models.ShadowCallAuthFailed, sa.ScrubText(firstErr)
	case okCalls == 0:
		return shadowConnectionFailed, sa.ScrubText(firstErr)
	default:
		return shadowConnectionPartial, sa.ScrubText(fmt.Sprintf("%d of %d calls failed; %s", failed, failed+okCalls, firstErr))
	}
}

// toShadowApply converts a connector result into what the repository
// stores, applying the retention rules once more: attributes through the
// allow-list, owner hints minimised, names scrubbed, and only evidence codes
// from the catalog.
func toShadowApply(
	scan *models.ShadowScan, c *models.ShadowConnection, out *sa.Result, startedAt, now time.Time, complete bool, log *slog.Logger,
) repositories.ShadowConnectionApply {
	in := repositories.ShadowConnectionApply{
		OUID: scan.OUID, ConnectionID: c.ID, ScanID: scan.ID, ScanStartedAt: startedAt, Complete: complete,
	}
	spec, _ := sa.Provider(c.Provider)
	for _, f := range out.Findings {
		if f == nil || strings.TrimSpace(f.ExternalID) == "" || !validShadowKind(spec.SourceHost, f.Kind) {
			continue
		}
		fi := repositories.ShadowFindingInput{
			Finding: models.ShadowFinding{
				Provider:          c.Provider,
				Kind:              f.Kind,
				ExternalID:        sa.Truncate(f.ExternalID, 1024),
				DisplayName:       sa.SanitizeName(firstNonEmpty(f.DisplayName, f.ExternalID)),
				Scope:             sa.NormaliseScope(c.Provider, c.Config.Organization, f.Scope),
				State:             sa.Truncate(f.State, 64),
				OwnerHint:         sa.MinimiseOwnerHint(f.OwnerHint),
				Attributes:        sa.SanitizeFindingAttributes(f.Kind, f.Attributes),
				ProviderCreatedAt: f.ProviderCreatedAt,
			},
		}
		for _, sig := range f.Signals {
			d, ok := sa.Def(sig.Code)
			if !ok || d.Provider != c.Provider {
				log.Warn("Dropping evidence unknown to the platform", "provider", c.Provider, "code", sig.Code)
				continue
			}
			fi.Signals = append(fi.Signals, models.ShadowFindingSignal{
				Code:       d.Code,
				Source:     sa.Truncate(firstNonEmpty(sig.Source, "scan"), 200),
				Observed:   sanitizeObserved(sig.Observed),
				DetectedAt: now,
			})
		}
		if len(fi.Signals) == 0 {
			// Every finding is an agent a platform listing or the source code
			// scan returned; one without that evidence is not stored.
			continue
		}
		in.Findings = append(in.Findings, fi)
	}
	return in
}

// validShadowKind reports whether a finding kind fits its platform: a source
// repository host finds code agents only, an agent platform lists platform
// agents and runtimes.
func validShadowKind(sourceHost bool, kind string) bool {
	if sourceHost {
		return kind == models.ShadowKindCodeAgent
	}
	return kind == models.ShadowKindPlatformAgent || kind == models.ShadowKindRuntime
}

// sanitizeObserved keeps counts, flags and short labels only.
func sanitizeObserved(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if len(out) == 20 {
			break
		}
		k = sa.Truncate(k, 64)
		switch x := v.(type) {
		case int, int32, int64, bool:
			out[k] = x
		case float64:
			if f := finiteOrNil(&x); f != nil {
				out[k] = *f
			}
		case float32:
			f64 := float64(x)
			if f := finiteOrNil(&f64); f != nil {
				out[k] = *f
			}
		case string:
			if len(x) <= 64 {
				out[k] = sa.ScrubText(x)
			}
		}
	}
	return out
}

func finiteOrNil(f *float64) *float64 {
	if f == nil || *f != *f || *f > 1e15 || *f < -1e15 {
		return nil
	}
	return f
}

// ---- correlation across connections ----

// correlate links agents of different connections that may be the same
// agent: a Foundry external agent and a same-named agent found elsewhere
// (Foundry stores no endpoint for external agents, so a name match is only a
// "possible duplicate", never merged).
func (s *shadowAgentScanner) correlate(ctx context.Context, ouID string) error {
	return correlateShadowOrg(ctx, s.repo, ouID, s.now().UTC())
}

// correlateShadowOrg recomputes the org's cross-connection relations, after a
// scan or a source code scan publish.
func correlateShadowOrg(ctx context.Context, repo repositories.ShadowAgentRepository, ouID string, now time.Time) error {
	findings, err := repo.ListOrgFindings(ctx, ouID)
	if err != nil {
		return fmt.Errorf("listing findings: %w", err)
	}
	relations, signals := CorrelateShadowFindings(findings, now)
	return repo.ApplyCorrelation(ctx, ouID, relations, signals)
}

// CorrelateShadowFindings computes cross-connection relations and the
// evidence they raise.
func CorrelateShadowFindings(
	findings []models.ShadowFinding, now time.Time,
) (map[uuid.UUID][]models.ShadowFindingRelation, map[uuid.UUID][]models.ShadowFindingSignal) {
	relations := map[uuid.UUID][]models.ShadowFindingRelation{}
	signals := map[uuid.UUID][]models.ShadowFindingSignal{}
	addSignal := func(id uuid.UUID, code string, observed map[string]any) {
		d, _ := sa.Def(code)
		for _, existing := range signals[id] {
			if existing.Code == code {
				return
			}
		}
		signals[id] = append(signals[id], models.ShadowFindingSignal{
			Code: d.Code, Source: repositories.ShadowCorrelationSource, Observed: observed, DetectedAt: now,
		})
	}

	// Foundry external agents: possible duplicates by normalized name.
	byName := map[string][]*models.ShadowFinding{}
	for i := range findings {
		f := &findings[i]
		// Code agents are named after code, not an endpoint, so they are not
		// matched against Foundry external agents.
		if f.Provider == models.ShadowProviderFoundry || f.Kind == models.ShadowKindCodeAgent {
			continue
		}
		if n := normalizeShadowName(f.DisplayName); n != "" {
			byName[n] = append(byName[n], f)
		}
	}
	for i := range findings {
		f := &findings[i]
		if f.Provider != models.ShadowProviderFoundry {
			continue
		}
		if kind, _ := f.Attributes["agentKind"].(string); kind != "external" {
			continue
		}
		matches := byName[normalizeShadowName(f.DisplayName)]
		for _, m := range matches {
			relations[f.ID] = append(relations[f.ID], models.ShadowFindingRelation{
				FindingID: m.ID.String(), Relation: "possible_duplicate", Reason: "same name; Foundry stores no endpoint for external agents",
			})
			relations[m.ID] = append(relations[m.ID], models.ShadowFindingRelation{
				FindingID: f.ID.String(), Relation: "possible_duplicate", Reason: "registered in Foundry as an external agent with the same name",
			})
		}
		if len(matches) > 0 {
			addSignal(f.ID, sa.SigFoundryPossibleDupOf, map[string]any{"matches": len(matches)})
		}
	}
	return relations, signals
}

// normalizeShadowName lower-cases a name and keeps letters and digits only.
func normalizeShadowName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
