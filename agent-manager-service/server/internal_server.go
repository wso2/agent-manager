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

package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
)

// InternalServer represents the internal HTTPS server for WebSocket and gateway internal APIs
type InternalServer struct {
	server  *http.Server
	cfg     *config.InternalServerConfig
	handler http.Handler
}

// NewInternalServer creates a new internal HTTPS server
func NewInternalServer(cfg *config.InternalServerConfig, handler http.Handler) *InternalServer {
	return &InternalServer{
		cfg:     cfg,
		handler: handler,
	}
}

// Start starts the internal HTTPS server with self-signed certificate
func (s *InternalServer) Start() error {
	if s.cfg.Port < 1 || s.cfg.Port > 65535 {
		return fmt.Errorf("invalid port: %d", s.cfg.Port)
	}

	// Build certificate paths
	certPath := filepath.Join(s.cfg.CertDir, "cert.pem")
	keyPath := filepath.Join(s.cfg.CertDir, "key.pem")

	slog.Info("initializing with certs", certPath, keyPath)

	var cert tls.Certificate

	// Try to load existing certificates first
	if _, certErr := os.Stat(certPath); certErr == nil {
		if _, keyErr := os.Stat(keyPath); keyErr == nil {
			loadedCert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err != nil {
				slog.Warn("Failed to load existing certificates", "error", err)
			} else {
				slog.Info("Using existing certificates", "certDir", s.cfg.CertDir)
				cert = loadedCert
			}
		}
	}

	// Generate new certificate if not loaded
	if cert.Certificate == nil {
		slog.Info("Generating self-signed certificate for internal server")
		// Ensure cert directory exists
		if err := os.MkdirAll(s.cfg.CertDir, 0o700); err != nil {
			return fmt.Errorf("failed to create cert directory: %w", err)
		}
		generatedCert, err := generateSelfSignedCert(certPath, keyPath)
		if err != nil {
			return fmt.Errorf("failed to generate self-signed certificate: %w", err)
		}
		cert = generatedCert
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	address := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.server = &http.Server{
		Addr:           address,
		Handler:        s.handler,
		TLSConfig:      tlsConfig,
		ReadTimeout:    time.Duration(s.cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:   time.Duration(s.cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:    time.Duration(s.cfg.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes: s.cfg.MaxHeaderBytes,
	}

	slog.Info("Starting internal HTTPS server",
		"address", fmt.Sprintf("https://localhost:%d", s.cfg.Port),
		"note", "Using self-signed certificate (browsers will show security warnings)")

	return s.server.ListenAndServeTLS("", "")
}

// Shutdown gracefully shuts down the server
func (s *InternalServer) Shutdown(shutdownCtx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(shutdownCtx)
}

// generateSelfSignedCert creates a self-signed certificate for development and saves it to disk
func generateSelfSignedCert(certPath, keyPath string) (tls.Certificate, error) {
	// Generate private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Agent Manager Service Dev"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour), // Valid for 1 year
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost"},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Create PEM blocks
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	// Save certificate and key to disk for persistence
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to save certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to save private key: %w", err)
	}
	slog.Info("Saved certificate", "certPath", certPath, "keyPath", keyPath)

	// Create TLS certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}
