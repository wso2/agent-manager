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

// Package gwinit writes the AI Gateway's configuration at "amp init ai-gateway".
package gwinit

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gatewaytoken"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/initfiles"
)

//go:embed templates/config.toml
var configTemplate string

// Paths of the gateway files, relative to the output directory. config.toml names the key and
// certificate by their path inside the gateway containers, which mount the directory at /amp/gw.
const (
	GatewayConfigFile = "config.toml"
	// RegistrationTokenFile holds the token the gateway connects with; Agent Manager reads it to
	// register the default gateway.
	RegistrationTokenFile = "registration-token"
	gatewayAESKeyFile     = "aesgcm-keys/default-aesgcm256-v1.bin"
	gatewayCertFile       = "listener-certs/default-listener.crt"
	gatewayKeyFile        = "listener-certs/default-listener.key"
)

// WriteGateway writes the AI Gateway's config.toml into dir, creating its encryption key, listener
// certificate and, unless cfg names one, its registration token on the first run and reusing them
// afterwards. A configured token replaces a kept one.
func WriteGateway(cfg *GatewayConfig, dir string) error {
	gwOwner, err := initfiles.ParseOwner(cfg.FileOwner)
	if err != nil {
		return fmt.Errorf("%sFILE_OWNER: %w", keyPrefix, err)
	}
	ampOwner, err := initfiles.ParseOwner(cfg.CoreOwner)
	if err != nil {
		return fmt.Errorf("AMP_FILE_OWNER: %w", err)
	}
	for _, sub := range []string{".", "aesgcm-keys", "listener-certs"} {
		if err := initfiles.ChownDir(filepath.Join(dir, sub), gwOwner); err != nil {
			return err
		}
	}
	if _, err := initfiles.WriteOnce(filepath.Join(dir, gatewayAESKeyFile), 0o600, gwOwner, func() ([]byte, error) {
		key := make([]byte, 32)
		_, err := rand.Read(key)
		return key, err
	}); err != nil {
		return fmt.Errorf("gateway encryption key: %w", err)
	}
	if err := writeListenerCert(dir, cfg.Host, gwOwner); err != nil {
		return fmt.Errorf("gateway listener certificate: %w", err)
	}

	tokenPath := filepath.Join(dir, RegistrationTokenFile)
	if cfg.RegistrationToken != "" {
		err = initfiles.WriteFile(tokenPath, []byte(cfg.RegistrationToken), 0o600, ampOwner)
	} else {
		var token []byte
		token, err = initfiles.WriteOnce(tokenPath, 0o600, ampOwner, func() ([]byte, error) {
			t, err := gatewaytoken.New()
			return []byte(t), err
		})
		cfg.RegistrationToken = strings.TrimSpace(string(token))
	}
	if err != nil {
		return fmt.Errorf("gateway registration token: %w", err)
	}
	if _, err := gatewaytoken.Prefix(cfg.RegistrationToken); err != nil {
		return fmt.Errorf("%s: %w", tokenPath, err)
	}

	content, err := RenderGatewayConfig(cfg)
	if err != nil {
		return err
	}
	return initfiles.WriteFile(filepath.Join(dir, GatewayConfigFile), content, 0o600, gwOwner)
}

// RenderGatewayConfig renders config.toml.
func RenderGatewayConfig(cfg *GatewayConfig) ([]byte, error) {
	tmpl, err := template.New("config.toml").Option("missingkey=error").
		Funcs(template.FuncMap{"q": initfiles.Quote}).Parse(configTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return nil, fmt.Errorf("render config.toml: %w", err)
	}
	return buf.Bytes(), nil
}

// writeListenerCert creates a self-signed certificate for the gateway's HTTPS listener once.
func writeListenerCert(dir, host string, o *initfiles.Owner) error {
	certPath, keyPath := filepath.Join(dir, gatewayCertFile), filepath.Join(dir, gatewayKeyFile)
	var keyPEM []byte
	if _, err := initfiles.WriteOnce(certPath, 0o644, o, func() ([]byte, error) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
		if err != nil {
			return nil, err
		}
		tmpl := &x509.Certificate{
			SerialNumber: serial,
			Subject:      pkix.Name{CommonName: host},
			DNSNames:     []string{host, "localhost"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().AddDate(0, 0, 825),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			return nil, err
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, err
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
	}); err != nil {
		return err
	}
	if keyPEM != nil {
		return initfiles.WriteFile(keyPath, keyPEM, 0o600, o)
	}
	return initfiles.Settle(keyPath, 0o600, o)
}

// GatewayConfig is everything the AI Gateway files depend on.
type GatewayConfig struct {
	// ControlPlaneHost is host:port of Agent Manager's gateway WebSocket listener.
	ControlPlaneHost string
	// RegistrationToken is the gateway's control-plane token; generated and kept when empty.
	RegistrationToken string
	AdminPassword     string
	AMPJWKSURL        string
	// ThunderIssuer and ThunderJWKSURL are the platform identity provider's token
	// issuer and key set, seeded as the ThunderKeyManager key manager. Both are
	// optional, but go together: without them the gateway trusts no platform
	// identity provider, so Agent Identity on MCP proxies cannot be used there.
	ThunderIssuer  string
	ThunderJWKSURL string
	// Host is the gateway's public host name (MCP auth metadata, listener certificate).
	Host         string
	ListenerPort int
	// FileOwner is the "uid:gid" the gateway's files are given, and CoreOwner
	// the one the registration token is given, which the core reads.
	FileOwner string
	CoreOwner string
}

// keyPrefix is how the module's settings are named in the environment.
const keyPrefix = "MODULE_AI_GATEWAY_"

// LoadGatewayConfig reads the AI Gateway inputs from the module's settings.
func LoadGatewayConfig(get func(key string) string) (*GatewayConfig, error) {
	r := &settings{get: get}
	cfg := &GatewayConfig{
		ControlPlaneHost:  r.required("CONTROLPLANE_HOST"),
		RegistrationToken: r.str("DEFAULT_GATEWAY_TOKEN"),
		AdminPassword:     r.required("ADMIN_PASSWORD"),
		AMPJWKSURL:        r.required("AMP_JWKS_URL"),
		ThunderIssuer:     strings.TrimRight(r.str("THUNDER_ISSUER"), "/"),
		ThunderJWKSURL:    r.str("THUNDER_JWKS_URL"),
		Host:              r.required("HOST"),
		ListenerPort:      r.port("LISTENER_PORT", 19080),
		FileOwner:         r.str("FILE_OWNER"),
	}
	if (cfg.ThunderIssuer == "") != (cfg.ThunderJWKSURL == "") {
		r.errs = append(r.errs, fmt.Errorf("%sTHUNDER_ISSUER and %sTHUNDER_JWKS_URL must be set together", keyPrefix, keyPrefix))
	}
	if cfg.RegistrationToken != "" {
		if _, err := gatewaytoken.Prefix(cfg.RegistrationToken); err != nil {
			r.errs = append(r.errs, fmt.Errorf("%sDEFAULT_GATEWAY_TOKEN: %w", keyPrefix, err))
		}
	}
	return cfg, errors.Join(r.errs...)
}

// settings reads the module's settings, collecting every problem so they are
// reported together.
type settings struct {
	get  func(string) string
	errs []error
}

func (r *settings) str(key string) string { return strings.TrimSpace(r.get(key)) }

func (r *settings) required(key string) string {
	v := r.str(key)
	if v == "" {
		r.errs = append(r.errs, fmt.Errorf("%s%s is required", keyPrefix, key))
	}
	return v
}

func (r *settings) port(key string, def int) int {
	v := r.str(key)
	if v == "" {
		return def
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		r.errs = append(r.errs, fmt.Errorf("%s%s: %q is not a port", keyPrefix, key, v))
		return def
	}
	return p
}
