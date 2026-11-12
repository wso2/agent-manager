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

package fileauth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// ErrInvalidCredentials is returned for both an unknown user and a wrong
// password. The two are deliberately indistinguishable to the caller: telling
// them apart turns the login endpoint into a username oracle.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Refusal reasons, recorded in the audit trail but never returned to the
// caller: telling an unknown user apart from a bad password is exactly the
// distinction ErrInvalidCredentials exists to hide.
const (
	reasonUnknownUser = "unknown-user"
	reasonBadPassword = "bad-password"
)

// dummyHash is compared against when no user matches, so that a request for an
// unknown username costs the same as one for a known username with the wrong
// password. Without it, response time alone enumerates valid usernames.
// It is a real bcrypt hash of an unguessable value, so the compare does the
// same work rather than failing fast on a malformed hash.
const dummyHash = "$2a$12$N9qo8uLOickgx2ZMRZoMye1VdLCQ1kK9pUjLcGZ3Q3K2Yy3kQ0h9W"

// Claims is the JWT a login mints.
//
// The field names match jwtassertion.TokenClaims exactly — ouId, ouHandle and
// a space-separated scope string — because that is the struct the middleware
// will parse this token back into. A mismatch here surfaces as a 403 with no
// indication that the claim was simply spelled differently.
type Claims struct {
	jwt.RegisteredClaims
	Scope    string   `json:"scope"`
	OuId     string   `json:"ouId"` //nolint:revive,stylecheck // matches jwtassertion.TokenClaims
	OuHandle string   `json:"ouHandle"`
	Roles    []string `json:"roles,omitempty"`
}

// Service authenticates users from the file and issues tokens for them.
type Service interface {
	// Login verifies the credentials and returns a signed token plus the
	// seconds until it expires.
	Login(ctx context.Context, username, password string) (token string, expiresIn int, err error)
	// Enabled reports whether file-based login is the configured auth mode.
	Enabled() bool
}

type service struct {
	cfg        config.FileAuthConfig
	users      *UserFile
	privateKey *rsa.PrivateKey
	keyID      string
	enabled    bool
	logger     *slog.Logger
}

// NewService builds the file-auth service.
//
// When AUTH_MODE is not "file" it returns a disabled service rather than an
// error: the route is still registered, so that a deployment configured for an
// IdP answers a login attempt with a clear 404 instead of a mystery.
//
// When AUTH_MODE *is* "file", every failure here is fatal. A service that
// starts without a usable user file or signing key cannot authenticate anyone,
// and failing at boot is the only place that is cheap to notice.
func NewService(cfg *config.Config, logger *slog.Logger) (Service, error) {
	if cfg.AuthMode != config.AuthModeFile {
		return &service{enabled: false, logger: logger}, nil
	}

	users, err := LoadUserFile(cfg.FileAuth.UsersPath)
	if err != nil {
		return nil, err
	}

	privateKey, err := loadPrivateKey(cfg.JWTSigning.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("file auth: %w", err)
	}

	logger.Info("file-based authentication enabled",
		"users", len(users.Users),
		"org", users.Organization.Handle,
		"issuer", cfg.FileAuth.Issuer,
		"kid", cfg.JWTSigning.ActiveKeyID,
		"tokenTTL", cfg.FileAuth.TokenTTL)

	return &service{
		cfg:        cfg.FileAuth,
		users:      users,
		privateKey: privateKey,
		keyID:      cfg.JWTSigning.ActiveKeyID,
		enabled:    true,
		logger:     logger,
	}, nil
}

func (s *service) Enabled() bool { return s.enabled }

func (s *service) Login(ctx context.Context, username, password string) (string, int, error) {
	if !s.enabled {
		return "", 0, ErrInvalidCredentials
	}

	var matched *User
	for i := range s.users.Users {
		if s.users.Users[i].Username == username {
			matched = &s.users.Users[i]
			break
		}
	}

	if matched == nil {
		// Spend the same time as a real compare before refusing. The result is
		// discarded; only the work matters.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		s.recordRefusal(ctx, username, reasonUnknownUser)
		return "", 0, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(matched.PasswordHash), []byte(password)); err != nil {
		s.recordRefusal(ctx, username, reasonBadPassword)
		return "", 0, ErrInvalidCredentials
	}

	now := time.Now()
	expiry := now.Add(s.cfg.TokenTTL)
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   matched.Username,
			Issuer:    s.cfg.Issuer,
			Audience:  jwt.ClaimStrings{s.cfg.Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
		Scope:    strings.Join(ScopesForRoles(matched.Roles), " "),
		OuId:     s.users.Organization.UUID,
		OuHandle: s.users.Organization.Handle,
		Roles:    slices.Clone(matched.Roles),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	// The JWKS path looks the key up by kid before it verifies anything, so a
	// token without one is refused before the signature is even checked.
	token.Header["kid"] = s.keyID

	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		s.logger.Error("file auth: failed to sign token", "error", err, "username", username)
		return "", 0, fmt.Errorf("sign token: %w", err)
	}

	expiresIn := int64(time.Until(expiry).Seconds())

	// Recorded before the token is handed back, on the same reasoning as agent
	// token minting: signing is local and stateless, so a token that is never
	// returned was never usable. Refusing here leaves no credential in
	// circulation, which makes this genuinely fail-closed rather than
	// best-effort. The signed token never reaches the recorder.
	if err := audit.RecordSync(ctx, audit.ActionAuthLogin,
		audit.Org(s.users.Organization.UUID),
		audit.ResourceNamed(audit.ResourceUser, matched.Username, matched.Username),
		audit.Detail("username", matched.Username),
		audit.Detail("roles", slices.Clone(matched.Roles)),
		audit.Detail("expiresIn", expiresIn),
	); err != nil {
		s.logger.Error("refusing to issue login token: audit record could not be written",
			"username", matched.Username, "error", err)
		return "", 0, err
	}

	s.logger.Info("file auth login succeeded",
		"username", matched.Username, "roles", matched.Roles, "expiresAt", expiry)
	return signed, int(expiresIn), nil
}

// recordRefusal logs and audits a refused sign-in.
//
// Fail-open, unlike the success path: a refusal issues no credential, so there
// is nothing in circulation for a missing record to hide. Blocking the refusal
// on the audit path would also hand an attacker a way to turn a recorder
// outage into a different response for a wrong password.
func (s *service) recordRefusal(ctx context.Context, username, reason string) {
	s.logger.Warn("file auth login failed", "reason", reason, "username", username)
	audit.Record(ctx, audit.ActionAuthLogin,
		audit.Org(s.users.Organization.UUID),
		audit.ResourceNamed(audit.ResourceUser, username, username),
		audit.Detail("username", username),
		audit.Detail("reason", reason),
		audit.Result(ErrInvalidCredentials),
	)
}

// ScopesForRoles expands role names to the full scope strings a token carries,
// de-duplicated and sorted.
//
// The "amp:" prefix is not decoration: requireScopes compares against
// rbac.Permission.Scope(), which is ResourceServer + ":" + the permission. A
// token carrying bare permission names holds, as far as the middleware is
// concerned, no permissions at all.
func ScopesForRoles(roles []string) []string {
	set := make(map[string]struct{})
	for _, role := range roles {
		for _, perm := range rbac.PredefinedRolePermissions[role] {
			set[perm.Scope()] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for scope := range set {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

// loadPrivateKey reads the same RSA private key the service signs agent tokens
// with. One key is enough: login tokens are told apart from agent tokens by
// their issuer and audience, not by which key signed them.
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read signing key %s: %w", path, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("decode signing key %s: not PEM", path)
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse signing key %s: %w", path, err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("signing key %s is %T, not RSA", path, parsed)
	}
	return key, nil
}
