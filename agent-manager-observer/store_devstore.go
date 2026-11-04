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

//go:build devstore

package main

import (
	"github.com/wso2/agent-manager/agent-manager-observer/app"
	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/devstore"
)

func newStoreOptions(cfg *config.Config) (app.Options, error) {
	client, ingest, ingestAddr, closeStore, err := devstore.New(cfg.Observer.DefaultNamespace)
	if err != nil {
		return app.Options{}, err
	}
	return app.Options{Client: client, Ingest: ingest, IngestAddr: ingestAddr, Close: closeStore}, nil
}
