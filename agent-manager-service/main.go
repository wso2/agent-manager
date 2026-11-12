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
	"log/slog"
	"os"

	"github.com/wso2/agent-manager/agent-manager-service/ampinit"
	"github.com/wso2/agent-manager/agent-manager-service/app"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/unsupported"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

func main() {
	// Parse command-line flags
	serverFlag := flag.Bool("server", true, "start the http Server")
	migrateFlag := flag.Bool("migrate", false, "migrate the database")
	waitForMigrationsFlag := flag.Bool("wait-for-migrations", false, "wait until the database migrations are applied")
	initThunderDir := flag.String("init-thunder", "", "write ThunderID's configuration into this directory and exit")
	initGatewayDir := flag.String("init-gateway", "", "write the AI Gateway's configuration into this directory and exit")
	flag.Parse()

	if *initThunderDir != "" || *initGatewayDir != "" {
		if err := ampinit.Run(*initThunderDir, *initGatewayDir); err != nil {
			slog.Error("writing component configuration failed", "error", err)
			os.Exit(1)
		}
		return
	}

	// No secret store is bundled yet; secret writes report ErrNotSupported.
	secretProvider := unsupported.NewProvider()

	// Open-source: DB-backed AgentID provisioning (env-Thunder credential read
	// from Postgres, not a key vault — see DBBackedAgentThunderProvisioning)
	agentThunderProvisioning := services.DBBackedAgentThunderProvisioning()

	app.Run(secretProvider, app.Options{
		Server:                   *serverFlag,
		Migrate:                  *migrateFlag,
		WaitForMigrations:        *waitForMigrationsFlag,
		AgentThunderProvisioning: agentThunderProvisioning,
	})
}
