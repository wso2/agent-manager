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
	"log/slog"
	"os"

	"github.com/wso2/agent-manager/agent-manager-observer/app"
	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}
	if err := cfg.Observer.ValidateClientCredentials(); err != nil {
		slog.Error("Invalid observer credentials config", "error", err)
		os.Exit(1)
	}

	authProvider := observer.NewAuthProvider(
		cfg.Observer.TokenURL,
		cfg.Observer.ClientID,
		cfg.Observer.ClientSecret,
	)
	if err := app.Run(cfg, authProvider, app.Options{}); err != nil {
		slog.Error("Observer stopped", "error", err)
		os.Exit(1)
	}
}
