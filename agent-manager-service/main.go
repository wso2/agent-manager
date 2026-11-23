// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/app"
	ocauth "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/auth"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/openchoreo"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

func main() {
	// Parse command-line flags
	serverFlag := flag.Bool("server", true, "start the http Server")
	migrateFlag := flag.Bool("migrate", false, "migrate the database")
	waitForMigrationsFlag := flag.Bool("wait-for-migrations", false, "wait until the database migrations are applied")
	healthcheckFlag := flag.Bool("healthcheck", false, "check the configured HTTP health endpoint and exit")
	flag.Parse()

	cfg := config.GetConfig()
	if *healthcheckFlag {
		if err := checkHealth(context.Background(), cfg.ServerHost, cfg.ServerPort); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Open-source: OAuth2 client credentials auth
	authProvider := ocauth.NewAuthProvider(ocauth.Config{
		TokenURL:     cfg.IDP.TokenURL,
		ClientID:     cfg.IDP.ClientID,
		ClientSecret: cfg.IDP.ClientSecret,
		Scope:        strings.Join(cfg.OAuthScopesSupported, " "),
	})

	// Secret management via the OpenChoreo secret management API
	secretProvider := openchoreo.NewProvider()

	// Open-source: DB-backed AgentID provisioning (env-Thunder credential read
	// from Postgres, not a key vault — see DBBackedAgentThunderProvisioning)
	agentThunderProvisioning := services.DBBackedAgentThunderProvisioning()

	app.Run(authProvider, secretProvider, app.Options{
		Server:                   *serverFlag,
		Migrate:                  *migrateFlag,
		WaitForMigrations:        *waitForMigrationsFlag,
		AgentThunderProvisioning: agentThunderProvisioning,
	})
}

// healthProbeIPs returns loopback literals for the configured listener. A
// family-agnostic wildcard bind can accept connections on either family.
func healthProbeIPs(ctx context.Context, host string, lookupIP func(context.Context, string, string) ([]net.IP, error)) ([]net.IP, error) {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") && strings.Contains(host, ":") {
		host = host[1 : len(host)-1]
	}
	ipv4Loopback, ipv6Loopback := net.IPv4(127, 0, 0, 1), net.IPv6loopback
	ip := net.ParseIP(host)
	if host == "" || ip.IsUnspecified() {
		if host == "" || ip.To4() != nil {
			return []net.IP{ipv4Loopback, ipv6Loopback}, nil
		}
		return []net.IP{ipv6Loopback, ipv4Loopback}, nil
	}

	if host == "localhost" {
		addrs, err := lookupIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve health check host: %w", err)
		}
		if len(addrs) > 0 {
			// Match the TCP listener's IPv4 preference for hostnames.
			ip = addrs[0]
			for _, addr := range addrs {
				if addr.To4() != nil {
					ip = addr
					break
				}
			}
		}
	}
	if !ip.IsLoopback() {
		return nil, fmt.Errorf("health check requires a loopback or wildcard server host")
	}
	return []net.IP{ip}, nil
}

// checkHealth probes only the local service. Plain HTTP is intentional: the
// process serves HTTP behind the ingress, and this request never leaves loopback.
func checkHealth(ctx context.Context, host string, port int) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ips, err := healthProbeIPs(ctx, host, net.DefaultResolver.LookupIP)
	if err != nil {
		return err
	}
	portText := strconv.Itoa(port)
	deadline, _ := ctx.Deadline()
	dialer := &net.Dialer{Deadline: deadline}
	client := &http.Client{
		Transport: &http.Transport{
			// Try the alternate family only when TCP cannot connect. HTTP
			// responses and redirects never trigger a second probe.
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				firstAddr := net.JoinHostPort(ips[0].String(), portText)
				if len(ips) == 1 {
					return dialer.DialContext(ctx, network, firstAddr)
				}
				// Reserve time for the other family if the first dial stalls.
				firstCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
				conn, err := dialer.DialContext(firstCtx, network, firstAddr)
				cancel()
				if err == nil {
					return conn, nil
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[1].String(), portText))
			},
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("health check redirects are disabled")
		},
	}
	defer client.CloseIdleConnections()
	healthURL := url.URL{Scheme: "http", Host: net.JoinHostPort(ips[0].String(), portText), Path: "/healthz"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build health check: %w", err)
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", response.StatusCode)
	}
	return nil
}
