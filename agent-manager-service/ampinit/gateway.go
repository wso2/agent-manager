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

package ampinit

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/gatewaytoken"
)

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
	gwOwner, err := parseOwner("GATEWAY_FILE_OWNER")
	if err != nil {
		return err
	}
	ampOwner, err := parseOwner("AMP_FILE_OWNER")
	if err != nil {
		return err
	}
	for _, sub := range []string{".", "aesgcm-keys", "listener-certs"} {
		if err := chownDir(filepath.Join(dir, sub), gwOwner); err != nil {
			return err
		}
	}
	if _, err := writeOnce(filepath.Join(dir, gatewayAESKeyFile), 0o600, gwOwner, func() ([]byte, error) {
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
		err = writeFile(tokenPath, []byte(cfg.RegistrationToken), 0o600, ampOwner)
	} else {
		var token []byte
		token, err = writeOnce(tokenPath, 0o600, ampOwner, func() ([]byte, error) {
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
	return writeFile(filepath.Join(dir, GatewayConfigFile), content, 0o600, gwOwner)
}

// RenderGatewayConfig renders config.toml.
func RenderGatewayConfig(cfg *GatewayConfig) ([]byte, error) {
	src, err := templates.ReadFile("templates/gateway/config.toml")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("config.toml").Option("missingkey=error").
		Funcs(template.FuncMap{"q": quote}).Parse(string(src))
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
func writeListenerCert(dir, host string, o *owner) error {
	certPath, keyPath := filepath.Join(dir, gatewayCertFile), filepath.Join(dir, gatewayKeyFile)
	var keyPEM []byte
	if _, err := writeOnce(certPath, 0o644, o, func() ([]byte, error) {
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
		return writeFile(keyPath, keyPEM, 0o600, o)
	}
	return settle(keyPath, 0o600, o)
}
