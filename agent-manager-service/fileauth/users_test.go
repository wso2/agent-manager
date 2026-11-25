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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func bcryptHash(t *testing.T) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	require.NoError(t, err)
	return string(h)
}

func TestLoadUserFileAccepts(t *testing.T) {
	path := writeFile(t, "users.yaml", `
organization: {handle: acme, uuid: 0192f0a0-0000-7000-8000-000000000001}
users:
  - {username: alice, passwordHash: "`+bcryptHash(t)+`", roles: ["`+rbac.RoleAdmin+`"]}
`)
	f, err := LoadUserFile(path)
	require.NoError(t, err)
	assert.Equal(t, "acme", f.Organization.Handle)
	require.Len(t, f.Users, 1)
	assert.Equal(t, []string{rbac.RoleAdmin}, f.Users[0].Roles)
}

// Every problem is reported at once, so an operator fixes the file in one pass.
func TestLoadUserFileReportsEveryProblem(t *testing.T) {
	path := writeFile(t, "users.yaml", `
organization: {handle: "", uuid: ""}
users:
  - {username: "", passwordHash: "", roles: []}
  - {username: bob, passwordHash: "not-bcrypt", roles: [Wizard]}
  - {username: bob, passwordHash: "`+bcryptHash(t)+`", roles: [Developer]}
`)
	_, err := LoadUserFile(path)
	require.Error(t, err)
	for _, want := range []string{
		"organization.handle is empty",
		"organization.uuid is empty",
		"users[0]: username is empty",
		"users[0]: passwordHash is empty",
		"users[0]: roles is empty",
		"users[1]: passwordHash is not a usable bcrypt hash",
		`users[1]: unknown role "Wizard"`,
		`users[2]: duplicate username "bob"`,
	} {
		assert.Contains(t, err.Error(), want)
	}
	for _, role := range KnownRoles() {
		assert.Contains(t, err.Error(), role, "the known roles are listed")
	}

	_, err = LoadUserFile(writeFile(t, "empty.yaml", "organization: {handle: a, uuid: b}\nusers: []\n"))
	require.ErrorContains(t, err, "nobody could log in")
	_, err = LoadUserFile(writeFile(t, "bad.yaml", "users: [unclosed"))
	require.ErrorContains(t, err, "parse user file")
	_, err = LoadUserFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorContains(t, err, "read user file")
}

func TestKnownRolesArePredefined(t *testing.T) {
	for _, role := range KnownRoles() {
		_, ok := rbac.PredefinedRolePermissions[role]
		assert.True(t, ok, role)
	}
}

func TestDisabledServiceRefusesEveryLogin(t *testing.T) {
	svc, err := NewService(&config.Config{AuthMode: config.AuthModeIDP}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	assert.False(t, svc.Enabled())
	_, _, err = svc.Login(context.Background(), "alice", "pw")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func pemFile(t *testing.T, blockType string, der []byte) string {
	t.Helper()
	return writeFile(t, "key.pem", string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})))
}

func TestNewServiceFailsAtBootWithoutUsableInputs(t *testing.T) {
	users := writeFile(t, "users.yaml", `
organization: {handle: acme, uuid: u}
users:
  - {username: alice, passwordHash: "`+bcryptHash(t)+`", roles: ["`+rbac.RoleDeveloper+`"]}
`)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	pkcs8RSA, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	require.NoError(t, err)

	build := func(usersPath, keyPath string) (Service, error) {
		cfg := &config.Config{AuthMode: config.AuthModeFile}
		cfg.FileAuth.UsersPath = usersPath
		cfg.JWTSigning.PrivateKeyPath = keyPath
		return NewService(cfg, slog.New(slog.DiscardHandler))
	}

	_, err = build(filepath.Join(t.TempDir(), "none.yaml"), pemFile(t, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey)))
	require.ErrorContains(t, err, "read user file")

	for name, tc := range map[string]struct {
		keyPath string
		want    string
	}{
		"missing key": {filepath.Join(t.TempDir(), "none.pem"), "read signing key"},
		"not PEM":     {writeFile(t, "k.pem", "plain text"), "not PEM"},
		"garbage DER": {pemFile(t, "PRIVATE KEY", []byte("junk")), "parse signing key"},
		"EC key":      {pemFile(t, "PRIVATE KEY", ecDER), "not RSA"},
	} {
		_, err := build(users, tc.keyPath)
		require.ErrorContains(t, err, tc.want, name)
	}

	// Both PKCS#1 and PKCS#8 RSA keys are accepted.
	for _, keyPath := range []string{
		pemFile(t, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey)),
		pemFile(t, "PRIVATE KEY", pkcs8RSA),
	} {
		svc, err := build(users, keyPath)
		require.NoError(t, err)
		assert.True(t, svc.Enabled())
		_, _, err = svc.Login(context.Background(), "alice", "wrong")
		require.ErrorIs(t, err, ErrInvalidCredentials)
	}
}
