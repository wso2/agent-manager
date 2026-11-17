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
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// Workload credentials are long-lived secrets a module hands its workloads,
// which exchange them for short-lived workload tokens. The secret is random,
// so a plain SHA-256 is enough to keep it out of the database.

const credentialSecretBytes = 32

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func (h *Host) issueCredential(ctx context.Context, module string, req modkit.CredentialRequest) (modkit.Credential, error) {
	if req.Subject == "" {
		return modkit.Credential{}, errors.New("modkit: a workload credential needs a subject")
	}
	if err := checkScopes(req.Scopes); err != nil {
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
	cred := modkit.Credential{ID: uuid.Must(uuid.NewV7()).String(), Secret: base64.RawURLEncoding.EncodeToString(buf)}
	// A credential nobody can account for must not exist: the intent is
	// recorded before the row is written, or nothing is written.
	opts := []audit.Option{
		audit.ResourceNamed("workload-credential", cred.ID, req.Subject),
		audit.Detail("module", module), audit.Detail("subject", req.Subject), audit.Detail("scopes", req.Scopes),
	}
	if org != "" {
		opts = append(opts, audit.Org(org))
	}
	attempt, err := audit.Begin(ctx, audit.ActionWorkloadCredentialIssue, opts...)
	if err != nil {
		return modkit.Credential{}, err
	}
	err = h.db.WithContext(ctx).Exec(`INSERT INTO workload_credentials
		(id, module, org_id, subject, scopes, token_ttl_seconds, secret_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		cred.ID, module, orgID, req.Subject, strings.Join(req.Scopes, " "),
		int(tokenTTL(req.TokenTTL)/time.Second), hashSecret(cred.Secret), dbx.Now()).Error
	attempt.Complete(ctx, err)
	if err != nil {
		return modkit.Credential{}, fmt.Errorf("storing workload credential: %w", err)
	}
	return cred, nil
}

func (h *Host) revokeCredential(ctx context.Context, module, id string) error {
	r := h.db.WithContext(ctx).Exec(`UPDATE workload_credentials SET revoked_at = ?
		WHERE id = ? AND module = ? AND revoked_at IS NULL`, dbx.Now(), id, module)
	audit.Record(ctx, audit.ActionWorkloadCredentialRevoke,
		audit.ResourceNamed("workload-credential", id, id), audit.Detail("module", module), audit.Result(r.Error))
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return modkit.ErrNotFound
	}
	return nil
}

type credentialRow struct {
	ID              string
	Module          string
	OrgID           *string
	Subject         string
	Scopes          string
	TokenTTLSeconds int
	SecretHash      string
}

// TokenExchangeRequest is the body of POST /workload-tokens.
type TokenExchangeRequest struct {
	CredentialID string `json:"credentialId"`
	Secret       string `json:"secret"`
}

// TokenExchangeResponse is its reply.
type TokenExchangeResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"tokenType"`
	ExpiresAt int64  `json:"expiresAt"`
}

// ServeTokenExchange exchanges a workload credential for a workload token. It
// is mounted at POST /workload-tokens on the internal API. A credential of a
// module that is not active is refused like an unknown one.
func (h *Host) ServeTokenExchange() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req TokenExchangeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil ||
			req.CredentialID == "" || req.Secret == "" {
			writeUnauthorized(w, http.StatusBadRequest, "credentialId and secret are required")
			return
		}
		tokens := h.deps().Tokens
		if tokens == nil {
			writeUnauthorized(w, http.StatusServiceUnavailable, "workload tokens are not available")
			return
		}
		var rows []credentialRow
		if _, err := uuid.Parse(req.CredentialID); err == nil {
			if err := h.db.WithContext(r.Context()).Raw(`SELECT id, module, org_id, subject, scopes, token_ttl_seconds, secret_hash
				FROM workload_credentials WHERE id = ? AND revoked_at IS NULL`, req.CredentialID).Scan(&rows).Error; err != nil {
				h.log.Error("modhost:reading workload credential failed", "error", err)
				writeUnauthorized(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
		if len(rows) != 1 || subtle.ConstantTimeCompare([]byte(rows[0].SecretHash), []byte(hashSecret(req.Secret))) != 1 ||
			!slices.Contains(h.current().active, rows[0].Module) {
			audit.Record(r.Context(), audit.ActionWorkloadTokenExchange,
				audit.OutcomeOpt(audit.OutcomeDeny), audit.Status(http.StatusUnauthorized),
				audit.Detail("credentialId", req.CredentialID))
			writeUnauthorized(w, http.StatusUnauthorized, "invalid workload credential")
			return
		}
		c := rows[0]
		wl := modkit.Workload{Module: c.Module, Subject: c.Subject, Scopes: strings.Fields(c.Scopes)}
		if c.OrgID != nil {
			wl.OrgID = *c.OrgID
		}
		tok, exp, err := tokens.IssueWorkload(wl, tokenTTL(time.Duration(c.TokenTTLSeconds)*time.Second))
		if err != nil {
			h.log.Error("modhost:signing workload token failed", "error", err)
			writeUnauthorized(w, http.StatusInternalServerError, "internal error")
			return
		}
		// Recorded before the token is handed back: a token that cannot be
		// accounted for is never issued.
		opts := []audit.Option{
			audit.Actor(audit.ActorService, "workload:"+c.Module+":"+c.Subject, ""),
			audit.Detail("module", c.Module), audit.Detail("credentialId", c.ID),
			audit.Detail("expiresIn", int64(time.Until(exp).Seconds())),
		}
		if wl.OrgID != "" {
			opts = append(opts, audit.Org(wl.OrgID))
		}
		if err := audit.RecordSync(r.Context(), audit.ActionWorkloadTokenExchange, opts...); err != nil {
			h.log.Error("modhost:refusing to issue workload token: audit record could not be written", "error", err)
			writeUnauthorized(w, http.StatusServiceUnavailable, "the token could not be recorded")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(TokenExchangeResponse{Token: tok, TokenType: "Bearer", ExpiresAt: exp.Unix()})
	})
}
