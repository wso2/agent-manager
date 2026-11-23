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
package cli

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/ampinit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/initfiles"
)

// firstRun generates what the server needs and nothing set: the secrets amp
// init generates, the encryption key, the JWT signing keys and, with file
// sign-in, the users file. Existing files are kept. hasData reports whether
// the database already holds an install, which a new encryption key would
// make unreadable.
func firstRun(ctx context.Context, hasData func(context.Context) (bool, error)) error {
	cfg := config.Unchecked()
	dir := config.SecretsDir()
	owner, err := initfiles.ParseOwner(config.Value("AMP_FILE_OWNER"))
	if err != nil {
		return fmt.Errorf("AMP_FILE_OWNER: %w", err)
	}
	if err := ampinit.EnsureSecrets(dir, owner); err != nil {
		return fmt.Errorf("generated secrets: %w", err)
	}
	if err := ensureEncryptionKey(ctx, dir, owner, hasData); err != nil {
		return err
	}
	if err := ensureSigningKeys(cfg.JWTSigning, owner); err != nil {
		return fmt.Errorf("JWT signing keys: %w", err)
	}
	if strings.EqualFold(cfg.AuthMode, config.AuthModeFile) {
		if err := ensureFileAuthUsers(cfg.FileAuth.UsersPath); err != nil {
			return fmt.Errorf("file sign-in users: %w", err)
		}
	}
	return nil
}

func ensureEncryptionKey(ctx context.Context, dir string, owner *initfiles.Owner, hasData func(context.Context) (bool, error)) error {
	if strings.TrimSpace(config.SetValue("ENCRYPTION_KEY")) != "" {
		return nil
	}
	path, _ := config.GeneratedFile("ENCRYPTION_KEY")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	existing, err := hasData(ctx)
	if err != nil {
		return fmt.Errorf("checking the database before generating ENCRYPTION_KEY: %w", err)
	}
	if existing {
		return fmt.Errorf("ENCRYPTION_KEY is not set and %s is missing, but the database already has data; "+
			"restore the key file or set ENCRYPTION_KEY (a new key would make stored secrets unreadable)", path)
	}
	return ampinit.EnsureServeSecret(dir, "ENCRYPTION_KEY", owner)
}

// ensureSigningKeys writes an RSA key pair and the public keys file the
// server reads, unless both files exist.
func ensureSigningKeys(c config.JWTSigningConfig, owner *initfiles.Owner) error {
	_, privErr := os.Stat(c.PrivateKeyPath)
	_, pubErr := os.Stat(c.PublicKeysConfigPath)
	switch {
	case privErr == nil && pubErr == nil:
		return nil
	case privErr == nil || pubErr == nil:
		return fmt.Errorf("only one of %s and %s exists; restore the other or remove both to generate a new pair",
			c.PrivateKeyPath, c.PublicKeysConfigPath)
	case !errors.Is(privErr, fs.ErrNotExist):
		return privErr
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	pubName := "public.pem"
	if c.ActiveKeyID != "" && c.ActiveKeyID != "key-1" {
		pubName = "public-" + c.ActiveKeyID + ".pem"
	}
	keys, err := json.MarshalIndent(config.PublicKeysConfig{Keys: []config.PublicKeyConfig{{
		Kid: c.ActiveKeyID, Algorithm: "RS256", PublicKeyPath: pubName,
		Description: "Initial JWT signing key", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}}}, "", "  ")
	if err != nil {
		return err
	}
	pubDir := filepath.Dir(c.PublicKeysConfigPath)
	if err := initfiles.WriteFile(filepath.Join(pubDir, pubName),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}), 0o644, owner); err != nil {
		return err
	}
	if err := initfiles.WriteFile(c.PublicKeysConfigPath, append(keys, '\n'), 0o644, owner); err != nil {
		return err
	}
	return initfiles.WriteFile(c.PrivateKeyPath,
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600, owner)
}

func ensureFileAuthUsers(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	cfg, err := ampinit.LoadFileAuthConfig()
	if err != nil {
		return err
	}
	if filepath.Base(path) != ampinit.FileAuthUsersFile {
		return fmt.Errorf("FILE_AUTH_USERS_PATH %s must be named %s for amp serve to write it", path, ampinit.FileAuthUsersFile)
	}
	return ampinit.WriteFileAuth(cfg, filepath.Dir(path))
}
