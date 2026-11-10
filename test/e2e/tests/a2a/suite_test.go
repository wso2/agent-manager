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

// Package a2a holds e2e tests for A2A agents reached through the gateway: a
// platform-hosted agent (platform_agent_test.go) and an external agent proxied
// to that platform agent's card (external_agent_test.go). One A2A sample agent
// is built for the whole suite.

package a2a

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

// Client is the shared API client used by all A2A tests.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

// Agent is the shared platform A2A agent, built once for the suite.
var Agent *framework.SharedA2AAgent

func TestA2A(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "A2A Suite")
}

var _ = SynchronizedBeforeSuite(testsetup.SynchronizedSharedA2AAgent(&Cfg, &Client, &Agent))
