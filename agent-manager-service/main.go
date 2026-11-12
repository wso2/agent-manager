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
	"flag"
	"log"
	"net"
	"net/http"
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
		host := cfg.ServerHost
		switch host {
		case "", "0.0.0.0":
			host = "127.0.0.1"
		case "::":
			host = "::1"
		}
		client := &http.Client{Timeout: 4 * time.Second}
		response, err := client.Get("http://" + net.JoinHostPort(host, strconv.Itoa(cfg.ServerPort)) + "/healthz")
		if err != nil {
			log.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			log.Fatalf("health check returned %s", response.Status)
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
