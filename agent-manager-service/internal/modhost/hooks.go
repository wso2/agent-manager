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

package modhost

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

var _ coreext.Extensions = (*Host)(nil)

// operation is one create, update or delete of a core resource. It holds the
// snapshot current when it began, so a module enabled or disabled meanwhile
// cannot get validate without apply, and the deadline all its hooks share.
type operation struct {
	h        *Host
	v        *view
	resource string
	deadline time.Time
}

// Begin starts an operation on a resource.
func (h *Host) Begin(_ context.Context, resource string) coreext.Operation {
	return &operation{h: h, v: h.current(), resource: resource, deadline: time.Now().Add(modkit.OperationTimeout)}
}

// Check rejects extension blocks addressed to a module that is not active or
// registered no hook on the resource.
func (op *operation) Check(ext map[string]json.RawMessage) error {
	names := make([]string, 0, len(ext))
	for name := range ext {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !slices.Contains(op.v.active, name) {
			return modkit.Reject("extensions.%s: no module named %q is active", name, name)
		}
		takes := false
		for _, phases := range op.v.hooks[op.resource] {
			for _, e := range phases {
				takes = takes || e.module == name
			}
		}
		if !takes {
			return modkit.Reject("extensions.%s: module %s takes no %s extensions", name, name, op.resource)
		}
	}
	return nil
}

// Run runs the hooks of one phase: validate and apply in activation order,
// finalize in reverse. Each hook sees only its own extension block and its own
// copy of the object.
func (op *operation) Run(ctx context.Context, tx *gorm.DB, phase modkit.Phase, req modkit.HookRequest, ext map[string]json.RawMessage) error {
	entries := op.v.hooks[op.resource][phase]
	if phase == modkit.Finalize {
		entries = slices.Clone(entries)
		slices.Reverse(entries)
	}
	req.Resource = op.resource
	for _, e := range entries {
		r := req
		r.Extension = ext[e.module]
		r.Object = deepCopy(req.Object)
		err := op.call(ctx, tx, e, r)
		if err == nil {
			continue
		}
		if phase == modkit.Finalize {
			// Finalize only cleans up; the delete goes ahead.
			op.h.log.Error("modhost:finalize hook failed; the delete goes ahead", "module", e.module, "resource", op.resource, "error", err)
			continue
		}
		return err
	}
	return nil
}

func deepCopy(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

// HookError is a hook's failure that is not a modkit.Error.
type HookError struct {
	Module, Resource string
	Phase            modkit.Phase
	Err              error
}

func (e *HookError) Error() string {
	return fmt.Sprintf("module %s: %s hook on %s: %v", e.Module, e.Phase, e.Resource, e.Err)
}

func (e *HookError) Unwrap() error { return e.Err }

type hookTxKey struct{}

// hookScope marks a hook's ctx. tx is the transaction the hook may write
// through; nil in the validate phase of a delete, which must only read.
type hookScope struct{ tx *gorm.DB }

// withHookTx marks ctx as a hook's, so Publish and IssueCredential called with
// it write through the core transaction instead of a second connection, or
// are refused where the hook has no transaction.
func withHookTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, hookTxKey{}, hookScope{tx: tx})
}

// hookTx returns the transaction of the hook ctx belongs to, and whether ctx
// is a hook's at all.
func hookTx(ctx context.Context) (*gorm.DB, bool) {
	s, ok := ctx.Value(hookTxKey{}).(hookScope)
	return s.tx, ok
}

var errReadOnlyHook = errors.New("modkit: a validate hook on a delete runs outside the transaction and cannot write; " +
	"publish or issue credentials from the finalize hook")

var savepoints atomic.Uint64

// call runs one hook. With a transaction, the hook runs inside its own
// savepoint, rolled back if the hook fails or runs out of time, so a module
// can neither leave partial writes nor leave the core transaction aborted.
// The hook's deadline never reaches the core's own statements: on PostgreSQL a
// cancelled statement closes the connection and with it the transaction, so
// there the hook's statements run uncancelled and the server's
// statement_timeout bounds them instead. On SQLite cancelling only interrupts
// the statement.
func (op *operation) call(ctx context.Context, tx *gorm.DB, e hookEntry, req modkit.HookRequest) error {
	deadline := time.Now().Add(modkit.HookTimeout)
	if op.deadline.Before(deadline) {
		deadline = op.deadline
	}
	hctx, cancel := context.WithDeadline(modkit.WithOrgID(ctx, req.OrgID), deadline)
	defer cancel()

	conn := &guardedConn{}
	var core *gorm.DB
	var sp string
	if tx != nil {
		core = tx.WithContext(context.WithoutCancel(ctx))
		sp = fmt.Sprintf("modhook_%d", savepoints.Add(1))
		if err := core.SavePoint(sp).Error; err != nil {
			return fmt.Errorf("module %s: opening a savepoint: %w", e.module, err)
		}
		stmtCtx := hctx
		if op.h.driver == "postgres" {
			ms := max(time.Until(deadline).Milliseconds(), 1)
			if err := core.Exec(fmt.Sprintf("SET LOCAL statement_timeout = %d", ms)).Error; err != nil {
				return fmt.Errorf("module %s: bounding the hook's statements: %w", e.module, err)
			}
			stmtCtx = context.WithoutCancel(hctx)
		}
		conn.inner = core.Statement.ConnPool
		req.Tx = guard(core.WithContext(stmtCtx), conn)
	}
	hctx = withHookTx(hctx, req.Tx)

	done := make(chan error, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- fmt.Errorf("panic: %v", p)
			}
		}()
		done <- e.fn(hctx, req)
	}()
	var err error
	select {
	case err = <-done:
	case <-hctx.Done():
		err = fmt.Errorf("did not finish in time: %w", hctx.Err())
	}
	// Waits for a statement the hook is running, then refuses any more. A
	// hook that ran out of time keeps running in its goroutine until it
	// returns, but can no longer touch the transaction.
	conn.close()

	if tx != nil {
		if err == nil {
			if op.h.driver == "postgres" {
				err = core.Exec("SET LOCAL statement_timeout TO DEFAULT").Error
			}
			if err == nil {
				err = core.Exec("RELEASE SAVEPOINT " + sp).Error
			}
		}
		if err != nil {
			if rbErr := core.RollbackTo(sp).Error; rbErr != nil {
				return fmt.Errorf("module %s: %w (and undoing its writes failed: %w)", e.module, err, rbErr)
			}
		}
	}
	if err == nil {
		return nil
	}
	var me *modkit.Error
	if errors.As(err, &me) {
		return &modkit.Error{Status: me.Status, Message: e.module + ": " + me.Message}
	}
	op.h.log.Error("modhost:hook failed", "module", e.module, "resource", op.resource, "phase", e.phase, "error", err)
	return &HookError{Module: e.module, Resource: op.resource, Phase: e.phase, Err: err}
}

// guard returns tx behind conn, which refuses every statement once closed, so
// a hook that outlives its call, or ran out of time, can no longer use the
// core transaction. Nested transactions use savepoints, as on the transaction
// itself; ending the transaction is refused.
func guard(tx *gorm.DB, conn *guardedConn) *gorm.DB {
	g := tx.Session(&gorm.Session{NewDB: true, Context: tx.Statement.Context})
	g.Statement.ConnPool = conn
	return g
}

var errHookTxClosed = errors.New("modkit: the hook's transaction is no longer usable")

// guardedConn holds its lock across the closed check and the statement, so
// close waits for a statement in progress and none starts after it. Rows a
// query returns are read after the lock is released; gorm reads them before
// the call that ran the query returns.
type guardedConn struct {
	inner  gorm.ConnPool
	mu     sync.Mutex
	closed bool
}

func (g *guardedConn) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

func (g *guardedConn) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, errHookTxClosed
	}
	return g.inner.PrepareContext(ctx, query)
}

func (g *guardedConn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, errHookTxClosed
	}
	return g.inner.ExecContext(ctx, query, args...)
}

func (g *guardedConn) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, errHookTxClosed
	}
	return g.inner.QueryContext(ctx, query, args...)
}

func (g *guardedConn) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		c, cancel := context.WithCancelCause(ctx)
		cancel(errHookTxClosed)
		ctx = c
	}
	return g.inner.QueryRowContext(ctx, query, args...)
}

// Commit and Rollback make gorm treat the handle as a transaction (nested
// Transaction calls become savepoints); a hook cannot end the core's.
func (g *guardedConn) Commit() error {
	return errors.New("modkit: a hook cannot commit the core transaction")
}

func (g *guardedConn) Rollback() error {
	return errors.New("modkit: a hook cannot roll back the core transaction")
}
