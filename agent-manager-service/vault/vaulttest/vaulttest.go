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

// Package vaulttest builds vaults for tests.
package vaulttest

import (
	"bytes"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

// Key is the fixed key the test vault seals with.
var Key = bytes.Repeat([]byte{0x2a}, vault.KeySize)

// Local returns the in-house vault under Key.
func Local(t testing.TB) *vault.Vaults {
	t.Helper()
	v, err := vault.NewLocal(Key)
	if err != nil {
		t.Fatalf("building the test vault: %v", err)
	}
	return vault.Of(v)
}
