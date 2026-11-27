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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dberr"
)

// Webhook secrets: the secrets developer portals sign their deliveries with, kept per organization. Each has a
// handle, the org.ref_id its portal sends (the portal organization's control-plane reference), and up to
// maxVersions versions, so a secret can be rotated while the portal still signs with the old one. A delivery
// names the version it was signed with by secret_hash; the secret's row says which organization it is for.
//
// So one organization here can take deliveries from several portals (a secret each), several organizations of
// one portal can each send to their own organization here, and a portal organization that also sends to another
// control plane can keep the reference that control plane needs (such as its organization handle): the secret
// maps it to the organization here.

const (
	// webhookSecretPrefix marks a generated secret, as the other control planes' secrets are marked.
	webhookSecretPrefix = "whsec_"
	webhookSecretBytes  = 32
	// DefaultWebhookSecretMaxVersions is how many versions a secret keeps unless webhook_secret_max_versions says.
	DefaultWebhookSecretMaxVersions = 3
	maxWebhookSecretNameChars       = 255
	maxWebhookOrgRefChars           = 255
	// maxLegacyCandidates bounds the secrets a delivery with no secret_hash is checked against, so an
	// unauthenticated caller can't make the receiver compute many signatures.
	maxLegacyCandidates = 32
)

var (
	errWebhookSecretNotFound      = errors.New("webhook secret not found")
	errWebhookSecretExists        = errors.New("a webhook secret with this handle exists")
	errWebhookSecretVersionsFull  = errors.New("the webhook secret has as many versions as it may keep")
	errWebhookSecretLastVersion   = errors.New("the last version of a webhook secret can't be deleted on its own")
	errWebhookSecretVersionAbsent = errors.New("webhook secret version not found")
)

// webhookSecret is a secret's row with its versions, newest first.
type webhookSecret struct {
	ID          string
	OrgID       string
	Handle      string
	DisplayName string
	OrgRef      *string
	LastVersion int
	CreatedBy   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Versions    []webhookSecretVersion `gorm:"-"`
}

type webhookSecretVersion struct {
	SecretID     string
	Version      int
	SecretHash   string
	MaskedSecret string
	CreatedBy    *string
	CreatedAt    time.Time
}

const (
	webhookSecretColumns        = `id, org_id, handle, display_name, org_ref, last_version, created_by, created_at, updated_at`
	webhookSecretVersionColumns = `secret_id, version, secret_hash, masked_secret, created_by, created_at`
)

// webhookSigner is a secret a delivery may be signed with, as the receiver uses it.
type webhookSigner struct {
	Secret string
	// OrgID and OrgRef are the organization the secret is for and the reference its portal sends. Both empty
	// for the module's own webhook_secret, which is for any organization a delivery names by its id.
	OrgID  string
	OrgRef string
}

// webhookSecrets is what the receiver reads of the secrets.
type webhookSecrets interface {
	// signerByHash returns the secret whose hash a delivery names, or found false.
	signerByHash(ctx context.Context, hash string) (signer webhookSigner, found bool, err error)
	// signersForRef returns the secrets a delivery with no secret_hash may be signed with: those whose portal
	// reference, or organization id, is ref. At most limit, newest first.
	signersForRef(ctx context.Context, ref string, limit int) ([]webhookSigner, error)
}

// webhookSecretService keeps the webhook secrets: their rows and versions in the module's tables, their values
// in the core's secret store.
type webhookSecretService struct {
	db          func() *gorm.DB
	secrets     func() modkit.Secrets
	maxVersions int
	log         *slog.Logger
	now         func() time.Time
	// generate returns a new secret value; tests replace it.
	generate func() (string, error)
}

// webhookSecretPath is where a version's value is kept in the core's secret store.
func webhookSecretPath(secretID string, version int) string {
	return "webhook-secrets/" + secretID + "/v" + strconv.Itoa(version)
}

// newWebhookSecretValue returns "whsec_" and 64 random hex characters.
func newWebhookSecretValue() (string, error) {
	b := make([]byte, webhookSecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate a webhook secret: %w", err)
	}
	return webhookSecretPrefix + hex.EncodeToString(b), nil
}

// maskWebhookSecret keeps the prefix and the last four characters.
func maskWebhookSecret(secret string) string {
	if len(secret) <= len(webhookSecretPrefix)+4 {
		return webhookSecretPrefix + "****"
	}
	return webhookSecretPrefix + "****" + secret[len(secret)-4:]
}

// webhookSecretInput is a new secret as the API takes it.
type webhookSecretInput struct {
	Handle      string
	DisplayName string
	OrgRef      string
}

func (in *webhookSecretInput) normalize() error {
	in.Handle = strings.TrimSpace(in.Handle)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.OrgRef = strings.TrimSpace(in.OrgRef)
	switch {
	case in.Handle == "":
		return invalid("id is required")
	case len(in.Handle) > maxHandleChars || !handlePattern.MatchString(in.Handle):
		return invalid("id must be at most %d lowercase letters, digits and hyphens, starting and ending with a "+
			"letter or digit", maxHandleChars)
	case len(in.DisplayName) > maxWebhookSecretNameChars:
		return invalid("name must be at most %d characters", maxWebhookSecretNameChars)
	case len(in.OrgRef) > maxWebhookOrgRefChars:
		return invalid("orgRef must be at most %d characters", maxWebhookOrgRefChars)
	}
	if in.DisplayName == "" {
		in.DisplayName = in.Handle
	}
	return nil
}

// create adds a secret with its first version, and returns it with the value, which is shown only now.
func (s *webhookSecretService) create(ctx context.Context, orgID, actor string, in webhookSecretInput) (*webhookSecret, string, error) {
	if err := in.normalize(); err != nil {
		return nil, "", err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("new webhook secret id: %w", err)
	}
	value, err := s.generate()
	if err != nil {
		return nil, "", err
	}
	now := s.now().UTC()
	var orgRef *string
	if in.OrgRef != "" {
		orgRef = &in.OrgRef
	}
	var created *webhookSecret
	err = s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO mod_ai_hub_webhook_secrets (`+webhookSecretColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.String(), orgID, in.Handle, in.DisplayName, orgRef, 1, nullable(actor), now, now).Error; err != nil {
			if dberr.IsDuplicateKey(err) {
				return fmt.Errorf("%w: %s", errWebhookSecretExists, in.Handle)
			}
			return fmt.Errorf("insert webhook secret: %w", err)
		}
		if err := s.addVersion(ctx, tx, orgID, id.String(), 1, value, actor, now); err != nil {
			return err
		}
		// Read back in the transaction, so the value, shown only now, always comes with what was committed.
		created, err = s.load(tx, orgID, in.Handle)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	s.log.InfoContext(ctx, "webhook secret created", "orgId", orgID, "handle", in.Handle)
	return created, value, nil
}

// addVersion stores a version's value in the secret store and its hash in the versions table.
func (s *webhookSecretService) addVersion(ctx context.Context, tx *gorm.DB, orgID, secretID string, version int, value, actor string, now time.Time) error {
	if err := tx.Exec(`INSERT INTO mod_ai_hub_webhook_secret_versions (org_id, `+webhookSecretVersionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		orgID, secretID, version, SecretHash(value), maskWebhookSecret(value), nullable(actor), now).Error; err != nil {
		return fmt.Errorf("insert webhook secret version: %w", err)
	}
	if err := s.secrets().In(tx).Put(ctx, orgID, webhookSecretPath(secretID, version), value); err != nil {
		return fmt.Errorf("store the webhook secret: %w", err)
	}
	return nil
}

// rotate adds a version, and returns the secret with the new value, which is shown only now. The old versions
// stay valid until they are deleted; a secret that has maxVersions refuses another (errWebhookSecretVersionsFull).
func (s *webhookSecretService) rotate(ctx context.Context, orgID, actor, handle string) (*webhookSecret, string, error) {
	value, err := s.generate()
	if err != nil {
		return nil, "", err
	}
	var rotated *webhookSecret
	err = s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.find(tx, orgID, handle)
		if err != nil {
			return err
		}
		// Taking the next version number first serializes concurrent rotations of one secret: the second waits
		// on this row and then counts this one's version.
		res := tx.Exec(`UPDATE mod_ai_hub_webhook_secrets SET last_version = last_version + 1, updated_at = ? `+
			`WHERE org_id = ? AND id = ?`, s.now().UTC(), orgID, row.ID)
		if res.Error != nil {
			return fmt.Errorf("take the next webhook secret version: %w", res.Error)
		}
		var next int
		if err := tx.Raw(`SELECT last_version FROM mod_ai_hub_webhook_secrets WHERE org_id = ? AND id = ?`,
			orgID, row.ID).Scan(&next).Error; err != nil {
			return fmt.Errorf("read the webhook secret version: %w", err)
		}
		var count int64
		if err := tx.Raw(`SELECT COUNT(*) FROM mod_ai_hub_webhook_secret_versions WHERE org_id = ? AND secret_id = ?`,
			orgID, row.ID).Scan(&count).Error; err != nil {
			return fmt.Errorf("count webhook secret versions: %w", err)
		}
		if int(count) >= s.maxVersions {
			return fmt.Errorf("%w (%d): delete an old version first", errWebhookSecretVersionsFull, s.maxVersions)
		}
		if err := s.addVersion(ctx, tx, orgID, row.ID, next, value, actor, s.now().UTC()); err != nil {
			return err
		}
		rotated, err = s.load(tx, orgID, handle)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	s.log.InfoContext(ctx, "webhook secret rotated", "orgId", orgID, "handle", handle)
	return rotated, value, nil
}

// deleteVersion removes one version. The last one can't be removed on its own: delete the secret instead.
func (s *webhookSecretService) deleteVersion(ctx context.Context, orgID, handle string, version int) error {
	return s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.find(tx, orgID, handle)
		if err != nil {
			return err
		}
		if err := s.lock(tx, orgID, row.ID); err != nil {
			return err
		}
		versions, err := s.versions(tx, orgID, row.ID)
		if err != nil {
			return err
		}
		found := false
		for _, v := range versions {
			found = found || v.Version == version
		}
		switch {
		case !found:
			return fmt.Errorf("%w: %s v%d", errWebhookSecretVersionAbsent, handle, version)
		case len(versions) == 1:
			return errWebhookSecretLastVersion
		}
		if err := tx.Exec(`DELETE FROM mod_ai_hub_webhook_secret_versions WHERE org_id = ? AND secret_id = ? AND version = ?`,
			orgID, row.ID, version).Error; err != nil {
			return fmt.Errorf("delete webhook secret version: %w", err)
		}
		if err := s.secrets().In(tx).Delete(ctx, orgID, webhookSecretPath(row.ID, version)); err != nil {
			return fmt.Errorf("delete the stored webhook secret: %w", err)
		}
		s.log.InfoContext(ctx, "webhook secret version deleted", "orgId", orgID, "handle", handle, "version", version)
		return nil
	})
}

// delete removes a secret with all its versions: deliveries signed with it are refused from now on.
func (s *webhookSecretService) delete(ctx context.Context, orgID, handle string) error {
	return s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.find(tx, orgID, handle)
		if err != nil {
			return err
		}
		if err := s.lock(tx, orgID, row.ID); err != nil {
			return err
		}
		versions, err := s.versions(tx, orgID, row.ID)
		if err != nil {
			return err
		}
		// The versions are deleted here too, not only by the foreign key: SQLite enforces it only when asked.
		if err := tx.Exec(`DELETE FROM mod_ai_hub_webhook_secret_versions WHERE org_id = ? AND secret_id = ?`,
			orgID, row.ID).Error; err != nil {
			return fmt.Errorf("delete webhook secret versions: %w", err)
		}
		if err := tx.Exec(`DELETE FROM mod_ai_hub_webhook_secrets WHERE org_id = ? AND id = ?`, orgID, row.ID).Error; err != nil {
			return fmt.Errorf("delete webhook secret: %w", err)
		}
		for _, v := range versions {
			if err := s.secrets().In(tx).Delete(ctx, orgID, webhookSecretPath(row.ID, v.Version)); err != nil {
				return fmt.Errorf("delete the stored webhook secret: %w", err)
			}
		}
		s.log.InfoContext(ctx, "webhook secret deleted", "orgId", orgID, "handle", handle)
		return nil
	})
}

// get returns a secret with its versions (no values).
func (s *webhookSecretService) get(ctx context.Context, orgID, handle string) (*webhookSecret, error) {
	return s.load(s.db().WithContext(ctx), orgID, handle)
}

// lock takes the secret's row for the rest of the transaction, as rotate does by taking the next version number:
// a concurrent rotation or deletion of the same secret waits, then reads the committed versions. Without it, two
// deletions of different versions could each see the other's version and leave the secret with none, and a
// deletion could miss a version rotated in meanwhile and leave its value in the secret store.
func (s *webhookSecretService) lock(tx *gorm.DB, orgID, secretID string) error {
	if err := tx.Exec(`UPDATE mod_ai_hub_webhook_secrets SET updated_at = ? WHERE org_id = ? AND id = ?`,
		s.now().UTC(), orgID, secretID).Error; err != nil {
		return fmt.Errorf("lock the webhook secret: %w", err)
	}
	return nil
}

// load returns a secret with its versions (no values), read with tx.
func (s *webhookSecretService) load(tx *gorm.DB, orgID, handle string) (*webhookSecret, error) {
	row, err := s.find(tx, orgID, handle)
	if err != nil {
		return nil, err
	}
	if row.Versions, err = s.versions(tx, orgID, row.ID); err != nil {
		return nil, err
	}
	return row, nil
}

// list returns the organization's secrets with their versions (no values), by handle.
func (s *webhookSecretService) list(ctx context.Context, orgID string) ([]webhookSecret, error) {
	tx := s.db().WithContext(ctx)
	var rows []webhookSecret
	if err := tx.Raw(`SELECT `+webhookSecretColumns+` FROM mod_ai_hub_webhook_secrets WHERE org_id = ? ORDER BY handle`,
		orgID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list webhook secrets: %w", err)
	}
	for i := range rows {
		var err error
		if rows[i].Versions, err = s.versions(tx, orgID, rows[i].ID); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (s *webhookSecretService) find(tx *gorm.DB, orgID, handle string) (*webhookSecret, error) {
	var rows []webhookSecret
	if err := tx.Raw(`SELECT `+webhookSecretColumns+` FROM mod_ai_hub_webhook_secrets WHERE org_id = ? AND handle = ?`,
		orgID, handle).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read webhook secret: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: %s", errWebhookSecretNotFound, handle)
	}
	return &rows[0], nil
}

func (s *webhookSecretService) versions(tx *gorm.DB, orgID, secretID string) ([]webhookSecretVersion, error) {
	var versions []webhookSecretVersion
	if err := tx.Raw(`SELECT `+webhookSecretVersionColumns+` FROM mod_ai_hub_webhook_secret_versions `+
		`WHERE org_id = ? AND secret_id = ? ORDER BY version DESC`, orgID, secretID).Scan(&versions).Error; err != nil {
		return nil, fmt.Errorf("read webhook secret versions: %w", err)
	}
	return versions, nil
}

// signerRow is a version joined with its secret, as the receiver needs it.
type signerRow struct {
	OrgID    string
	OrgRef   *string
	SecretID string
	Version  int
}

func (s *webhookSecretService) signerFrom(ctx context.Context, r signerRow) (webhookSigner, error) {
	value, err := s.secrets().Get(ctx, r.OrgID, webhookSecretPath(r.SecretID, r.Version))
	if err != nil {
		return webhookSigner{}, fmt.Errorf("read the stored webhook secret: %w", err)
	}
	signer := webhookSigner{Secret: value, OrgID: r.OrgID}
	if r.OrgRef != nil {
		signer.OrgRef = *r.OrgRef
	}
	return signer, nil
}

func (s *webhookSecretService) signerByHash(ctx context.Context, hash string) (webhookSigner, bool, error) {
	var rows []signerRow
	if err := s.db().WithContext(ctx).Raw(`SELECT v.org_id, w.org_ref, v.secret_id, v.version `+
		`FROM mod_ai_hub_webhook_secret_versions v JOIN mod_ai_hub_webhook_secrets w ON w.org_id = v.org_id AND w.id = v.secret_id `+
		`WHERE v.secret_hash = ?`, hash).Scan(&rows).Error; err != nil {
		return webhookSigner{}, false, fmt.Errorf("find the webhook secret by hash: %w", err)
	}
	if len(rows) == 0 {
		return webhookSigner{}, false, nil
	}
	signer, err := s.signerFrom(ctx, rows[0])
	if err != nil {
		return webhookSigner{}, false, err
	}
	return signer, true, nil
}

// signersForRef returns, newest first and at most limit, the secrets a delivery with no secret_hash naming ref may be
// signed with. When ref is an organization's id, that organization's own secrets come first, with the whole budget:
// secrets other organizations map to the same reference (orgRef) only take what is left, so they can't crowd them out.
func (s *webhookSecretService) signersForRef(ctx context.Context, ref string, limit int) ([]webhookSigner, error) {
	const signerQuery = `SELECT v.org_id, w.org_ref, v.secret_id, v.version ` +
		`FROM mod_ai_hub_webhook_secret_versions v JOIN mod_ai_hub_webhook_secrets w ON w.org_id = v.org_id AND w.id = v.secret_id `
	db := s.db().WithContext(ctx)
	var rows []signerRow
	if validID(ref) {
		if err := db.Raw(signerQuery+`WHERE v.org_id = ? ORDER BY v.created_at DESC LIMIT ?`, ref, limit).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("find the webhook secrets of %s: %w", ref, err)
		}
	}
	if left := limit - len(rows); left > 0 {
		// The organization's own secrets are already in rows; org_id is compared only with an id, which a
		// uuid column on PostgreSQL requires.
		q := db.Raw(signerQuery+`WHERE w.org_ref = ? ORDER BY v.created_at DESC LIMIT ?`, ref, left)
		if validID(ref) {
			q = db.Raw(signerQuery+`WHERE w.org_ref = ? AND v.org_id <> ? ORDER BY v.created_at DESC LIMIT ?`, ref, ref, left)
		}
		var mapped []signerRow
		if err := q.Scan(&mapped).Error; err != nil {
			return nil, fmt.Errorf("find the webhook secrets mapped to %s: %w", ref, err)
		}
		rows = append(rows, mapped...)
	}
	signers := make([]webhookSigner, 0, len(rows))
	for _, r := range rows {
		signer, err := s.signerFrom(ctx, r)
		if err != nil {
			return nil, err
		}
		signers = append(signers, signer)
	}
	return signers, nil
}
