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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// Workload credentials are long-lived secrets a module's service workloads
// present in their heartbeats, which return short-lived workload tokens. The
// secret is random, so a plain SHA-256 is enough to keep it out of the
// database. A workload sees one opaque string, "<id>.<secret>": the id finds
// the row and the secret is compared with its hash.

const credentialSecretBytes = 32

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// splitCredential splits what a workload presents into the credential id and
// its secret.
func splitCredential(s string) (id, secret string, ok bool) {
	id, secret, ok = strings.Cut(s, ".")
	if !ok || secret == "" {
		return "", "", false
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", "", false
	}
	return id, secret, true
}

// serviceWorkload returns a service workload a module declares.
func (h *Host) serviceWorkload(module, name string) (modkit.WorkloadSpec, error) {
	for _, wl := range h.manifests[module].Workloads {
		if wl.Name != name {
			continue
		}
		if wl.Kind != modkit.WorkloadService {
			return wl, fmt.Errorf("modkit: %s/%s is a %s workload; only service workloads get credentials", module, name, wl.Kind)
		}
		return wl, nil
	}
	return modkit.WorkloadSpec{}, fmt.Errorf("modkit: module %s declares no workload %q", module, name)
}

// IssueCredential mints a credential for a service workload of an active
// module on behalf of an operator (`amp module credential`). The secret is
// returned only here.
func (h *Host) IssueCredential(ctx context.Context, module, workload, subject string) (modkit.Credential, error) {
	if _, ok := h.mods[module]; !ok {
		return modkit.Credential{}, &ResolveError{Msg: module + " is not compiled into this binary"}
	}
	if _, err := h.serviceWorkload(module, workload); err != nil {
		return modkit.Credential{}, &ResolveError{Msg: strings.TrimPrefix(err.Error(), "modkit: ")}
	}
	h.mu.RLock()
	st := stateOf(module, h.states)
	h.mu.RUnlock()
	if st != StateActive {
		return modkit.Credential{}, &ResolveError{Msg: fmt.Sprintf("%s is %s, not active", module, st), Hint: "install or enable it first"}
	}
	return h.issueCredential(ctx, module, modkit.CredentialRequest{Workload: workload, Subject: subject})
}

func (h *Host) issueCredential(ctx context.Context, module string, req modkit.CredentialRequest) (modkit.Credential, error) {
	if req.Subject == "" || len(req.Subject) > 255 || strings.ContainsAny(req.Subject, " \t\r\n") {
		return modkit.Credential{}, errors.New("modkit: a workload credential needs a subject of at most 255 characters, without spaces")
	}
	if tx, inHook := hookTx(ctx); inHook && tx == nil {
		return modkit.Credential{}, errReadOnlyHook
	}
	if _, err := h.serviceWorkload(module, req.Workload); err != nil {
		return modkit.Credential{}, err
	}
	org := req.OrgID
	if org == "" {
		org = modkit.OrgID(ctx)
	}
	var orgID any
	if org != "" {
		id, err := uuid.Parse(org)
		if err != nil {
			return modkit.Credential{}, fmt.Errorf("modkit: organization id %q is not valid", org)
		}
		orgID = id
	}
	buf := make([]byte, credentialSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return modkit.Credential{}, err
	}
	id := uuid.Must(uuid.NewV7()).String()
	secret := base64.RawURLEncoding.EncodeToString(buf)
	// A credential nobody can account for must not exist: the intent is
	// recorded before the row is written, or nothing is written.
	opts := []audit.Option{
		audit.ResourceNamed("workload-credential", id, req.Subject),
		audit.Detail("module", module), audit.Detail("workload", req.Workload), audit.Detail("subject", req.Subject),
	}
	if org != "" {
		opts = append(opts, audit.Org(org))
	}
	attempt, err := audit.Begin(ctx, audit.ActionWorkloadCredentialIssue, opts...)
	if err != nil {
		return modkit.Credential{}, err
	}
	db, _ := hookTx(ctx)
	if db == nil {
		db = h.db
	}
	err = db.WithContext(ctx).Exec(`INSERT INTO workload_credentials
		(id, module, workload, org_id, subject, secret_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, module, req.Workload, orgID, req.Subject, hashSecret(secret), dbx.Now()).Error
	attempt.Complete(ctx, err)
	if err != nil {
		return modkit.Credential{}, fmt.Errorf("storing workload credential: %w", err)
	}
	return modkit.Credential{ID: id, Secret: id + "." + secret}, nil
}

// revokeCredential revokes one of a module's credentials and forgets its
// instances, so module health stops counting them at once. Tokens already
// issued on it stay valid until they expire; the next heartbeat is refused.
func (h *Host) revokeCredential(ctx context.Context, module, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return modkit.ErrNotFound
	}
	var affected int64
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := tx.Exec(`UPDATE workload_credentials SET revoked_at = ?
			WHERE id = ? AND module = ? AND revoked_at IS NULL`, dbx.Now(), id, module)
		if r.Error != nil {
			return r.Error
		}
		affected = r.RowsAffected
		return tx.Exec(`DELETE FROM workload_instances WHERE credential_id = ?`, id).Error
	})
	audit.Record(ctx, audit.ActionWorkloadCredentialRevoke,
		audit.ResourceNamed("workload-credential", id, id), audit.Detail("module", module), audit.Result(err))
	if err != nil {
		return err
	}
	if affected == 0 {
		return modkit.ErrNotFound
	}
	return nil
}

// RevokeAllCredentials revokes every credential of a module and forgets its
// workload instances. Purging a module's data calls it, so a reinstalled
// module starts with no workload that can still sign in.
func (h *Host) RevokeAllCredentials(ctx context.Context, module string) (int, error) {
	var ids []string
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT id FROM workload_credentials WHERE module = ? AND revoked_at IS NULL ORDER BY id`, module).
			Scan(&ids).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE workload_credentials SET revoked_at = ? WHERE module = ? AND revoked_at IS NULL`,
			dbx.Now(), module).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM workload_instances WHERE module = ?`, module).Error
	})
	for _, id := range ids {
		audit.Record(ctx, audit.ActionWorkloadCredentialRevoke,
			audit.ResourceNamed("workload-credential", id, id), audit.Detail("module", module), audit.Result(err))
	}
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// activeModule reports whether module is active in this replica's snapshot.
func (h *Host) activeModule(module string) bool {
	return slices.Contains(h.current().active, module)
}

// RevokeCredential revokes one of a module's credentials on behalf of an
// operator.
func (h *Host) RevokeCredential(ctx context.Context, module, id string) error {
	return h.revokeCredential(ctx, module, id)
}
