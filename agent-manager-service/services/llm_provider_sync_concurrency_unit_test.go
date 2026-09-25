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

package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// Overlapping provider edits each spawn a detached sync. Without serialisation the
// slower one can read valid state and then write it after the faster one, leaving a
// proxy unsecured while its provider requires a key. The lock must make the read and
// the walk one critical section per provider, so syncs never interleave.
func TestSyncDependentProxyAuthHeaders_SerialisesPerProvider(t *testing.T) {
	providerUUID := uuid.New()
	enabled := true
	stored := &models.LLMProvider{
		UUID: providerUUID,
		Configuration: models.LLMProviderConfig{
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey:  &models.APIKeySecurity{Enabled: &enabled, Key: "API-Key", In: "header"},
			},
		},
	}

	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0

	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(providerID, ouID string) (*models.LLMProvider, error) {
			mu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			mu.Unlock()
			// Hold the window open: without it the goroutines finish too fast to
			// overlap and the test would pass even with no serialisation at all.
			time.Sleep(5 * time.Millisecond)
			return stored, nil
		},
	}
	proxyRepo := &repomocks.LLMProxyRepositoryMock{
		ListByProviderFunc: func(ouID, providerUUID string, limit, offset int) ([]*models.LLMProxy, error) {
			mu.Lock()
			inFlight--
			mu.Unlock()
			return nil, nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo, proxyRepo: proxyRepo}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.SyncDependentProxyAuthHeaders(
				context.Background(), stored, "ou-acme", &LLMProxyService{}, &LLMProxyDeploymentService{})
		}()
	}
	wg.Wait()

	require.Equal(t, 1, maxInFlight,
		"syncs for one provider must not overlap: an older one can otherwise overwrite a newer auth requirement")
}

// The lock is per provider, so unrelated providers still sync concurrently.
func TestSyncDependentProxyAuthHeaders_DifferentProvidersDoNotBlock(t *testing.T) {
	svc := &LLMProviderService{}
	releaseA := svc.lockProviderSync("provider-a")
	defer releaseA()

	done := make(chan struct{})
	go func() {
		release := svc.lockProviderSync("provider-b")
		release()
		close(done)
	}()

	select {
	case <-done:
	case <-context.Background().Done():
		t.Fatal("a second provider's sync must not wait on the first")
	}
}
