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

	"github.com/wso2/agent-manager/agent-manager-service/app"
	ocauth "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/auth"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/openbao"
	"github.com/wso2/agent-manager/agent-manager-service/config"
)

func main() {
	// Parse command-line flags
	serverFlag := flag.Bool("server", true, "start the http Server")
	migrateFlag := flag.Bool("migrate", false, "migrate the database")
	flag.Parse()

	cfg := config.GetConfig()

	// Open-source: OAuth2 client credentials auth
	authProvider := ocauth.NewAuthProvider(ocauth.Config{
		TokenURL:     cfg.IDP.TokenURL,
		ClientID:     cfg.IDP.ClientID,
		ClientSecret: cfg.IDP.ClientSecret,
	})

	// Open-source: OpenBao secret management
	secretProvider := openbao.NewProvider()

	app.Run(authProvider, secretProvider, app.Options{
		Server:  *serverFlag,
		Migrate: *migrateFlag,
	})
}
