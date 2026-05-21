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

package traces

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestRequireEnvironment_NotFoundReturnsServerError(t *testing.T) {
	client, closeFn := newAMTestClient(t, http.StatusNotFound)
	defer closeFn()

	err := requireEnvironment(context.Background(), client, "acme", "nonexistent")
	if err == nil {
		t.Fatal("requireEnvironment should return an error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "Environment not found") {
		t.Errorf("error should mention 'Environment not found', got %q", err.Error())
	}
}

func TestRequireEnvironment_OK(t *testing.T) {
	client, closeFn := newAMTestClient(t, http.StatusOK)
	defer closeFn()

	if err := requireEnvironment(context.Background(), client, "acme", "dev"); err != nil {
		t.Fatalf("requireEnvironment should succeed for 200, got %v", err)
	}
}
