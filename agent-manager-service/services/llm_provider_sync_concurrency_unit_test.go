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
	"errors"
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
	releaseA, err := svc.lockProviderSync(context.Background(), "provider-a")
	require.NoError(t, err)
	defer releaseA()

	done := make(chan struct{})
	go func() {
		release, err := svc.lockProviderSync(context.Background(), "provider-b")
		if err != nil {
			return
		}
		release()
		close(done)
	}()

	select {
	case <-done:
	case <-context.Background().Done():
		t.Fatal("a second provider's sync must not wait on the first")
	}
}

// fakeProviderSyncDBLocker stands in for the database half of lockProviderSync.
// onLock and onRelease run while lockProviderSync holds or is dropping the
// in-process lock.
type fakeProviderSyncDBLocker struct {
	err       error
	onLock    func(providerUUID string)
	onRelease func(providerUUID string)
}

func (f *fakeProviderSyncDBLocker) LockProviderSync(_ context.Context, providerUUID string) (func(), error) {
	if f.onLock != nil {
		f.onLock(providerUUID)
	}
	if f.err != nil {
		return nil, f.err
	}
	return func() {
		if f.onRelease != nil {
			f.onRelease(providerUUID)
		}
	}, nil
}

// lockingProviderRepo is a provider repository that can also take sync locks,
// as the real LLMProviderRepo can.
type lockingProviderRepo struct {
	*repomocks.LLMProviderRepositoryMock
	fakeProviderSyncDBLocker
}

// providerSyncLockHeld reports whether providerUUID's in-process sync lock is held.
func providerSyncLockHeld(svc *LLMProviderService, providerUUID string) bool {
	v, ok := svc.proxySyncLocks.Load(providerUUID)
	if !ok {
		return false
	}
	mu := v.(*sync.Mutex)
	if mu.TryLock() {
		mu.Unlock()
		return false
	}
	return true
}

func TestNewLLMProviderService_UsesRepoSyncLocker(t *testing.T) {
	repo := &lockingProviderRepo{LLMProviderRepositoryMock: &repomocks.LLMProviderRepositoryMock{}}
	svc := NewLLMProviderService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NotNil(t, svc.proxySyncDBLocker, "a repository that can lock syncs must back lockProviderSync across replicas")

	plain := NewLLMProviderService(nil, &repomocks.LLMProviderRepositoryMock{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.Nil(t, plain.proxySyncDBLocker, "a repository that cannot lock syncs leaves only the in-process lock")
}

// The in-process lock comes first so syncs on one replica queue in memory and
// only the holder asks the database for the lock (and a connection).
func TestLockProviderSync_TakesInProcessLockBeforeDBLockAndReleasesInReverse(t *testing.T) {
	const providerUUID = "provider-a"
	svc := &LLMProviderService{}
	var dbLockedUUID string
	var heldAtDBLock, heldAtDBRelease bool
	svc.proxySyncDBLocker = &fakeProviderSyncDBLocker{
		onLock: func(id string) {
			dbLockedUUID = id
			heldAtDBLock = providerSyncLockHeld(svc, providerUUID)
		},
		onRelease: func(string) { heldAtDBRelease = providerSyncLockHeld(svc, providerUUID) },
	}

	release, err := svc.lockProviderSync(context.Background(), providerUUID)
	require.NoError(t, err)
	release()

	require.Equal(t, providerUUID, dbLockedUUID, "the database lock must use the provider UUID")
	require.True(t, heldAtDBLock, "the in-process lock must already be held when the database lock is requested")
	require.True(t, heldAtDBRelease, "the in-process lock must still be held when the database lock is released")
	require.False(t, providerSyncLockHeld(svc, providerUUID), "release must free the in-process lock")
}

func TestLockProviderSync_DBLockErrorReleasesInProcessLock(t *testing.T) {
	const providerUUID = "provider-a"
	dbErr := errors.New("database unavailable")
	svc := &LLMProviderService{proxySyncDBLocker: &fakeProviderSyncDBLocker{err: dbErr}}

	release, err := svc.lockProviderSync(context.Background(), providerUUID)
	require.ErrorIs(t, err, dbErr)
	require.Nil(t, release)
	require.False(t, providerSyncLockHeld(svc, providerUUID), "a failed database lock must not leave the in-process lock held")
}

// A sync that cannot take the lock must not touch the provider's proxies.
func TestSyncDependentProxyAuthHeaders_LockErrorSkipsSync(t *testing.T) {
	dbErr := errors.New("database unavailable")
	providerRepo := &repomocks.LLMProviderRepositoryMock{}
	svc := &LLMProviderService{providerRepo: providerRepo, proxySyncDBLocker: &fakeProviderSyncDBLocker{err: dbErr}}

	err := svc.SyncDependentProxyAuthHeaders(
		context.Background(), &models.LLMProvider{UUID: uuid.New()}, "ou-acme", &LLMProxyService{}, &LLMProxyDeploymentService{})

	require.ErrorIs(t, err, dbErr)
	require.Empty(t, providerRepo.GetByUUIDCalls(), "a sync that cannot lock must not read or rewrite anything")
}
