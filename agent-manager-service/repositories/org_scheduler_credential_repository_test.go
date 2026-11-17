//go:build integration

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

package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
)

func newOrgCredentialLocker(t *testing.T, pool *gorm.DB) OrgCredentialLocker {
	t.Helper()
	locker, ok := NewOrgSchedulerCredentialRepo(pool).(OrgCredentialLocker)
	require.True(t, ok, "the scheduler credential repository must implement OrgCredentialLocker")
	return locker
}

// lockOrgCredentialsAsync calls LockOrgCredentials on its own goroutine and
// delivers the release func, or closes the channel on error.
func lockOrgCredentialsAsync(locker OrgCredentialLocker, ouID string) <-chan func() {
	acquired := make(chan func(), 1)
	go func() {
		release, err := locker.LockOrgCredentials(context.Background(), ouID)
		if err != nil {
			close(acquired)
			return
		}
		acquired <- release
	}()
	return acquired
}

// TestOrgSchedulerCredentialRepo_LockOrgCredentials_SerializesSameOrg stands in
// for two replicas provisioning one org: only the database can make the second wait.
func TestOrgSchedulerCredentialRepo_LockOrgCredentials_SerializesSameOrg(t *testing.T) {
	locker := newOrgCredentialLocker(t, db.GetDB())
	ouID := "org-" + uuid.NewString()

	release, err := locker.LockOrgCredentials(context.Background(), ouID)
	require.NoError(t, err)

	waiter := lockOrgCredentialsAsync(locker, ouID)
	select {
	case r, ok := <-waiter:
		if ok {
			r()
		}
		t.Fatal("a second lock on the same org must wait for the holder")
	case <-time.After(300 * time.Millisecond):
	}

	release()
	select {
	case r, ok := <-waiter:
		require.True(t, ok, "the waiter must get the lock once the holder releases")
		r()
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter must get the lock once the holder releases")
	}
}

func TestOrgSchedulerCredentialRepo_LockOrgCredentials_DifferentOrgsDoNotBlock(t *testing.T) {
	locker := newOrgCredentialLocker(t, db.GetDB())
	release, err := locker.LockOrgCredentials(context.Background(), "org-"+uuid.NewString())
	require.NoError(t, err)
	defer release()

	select {
	case r, ok := <-lockOrgCredentialsAsync(locker, "org-"+uuid.NewString()):
		require.True(t, ok)
		r()
	case <-time.After(5 * time.Second):
		t.Fatal("another org's lock must not wait behind an unrelated one")
	}
}

// TestOrgSchedulerCredentialRepo_LockOrgCredentials_HolderCanUseCappedPool:
// with DB_MAX_OPEN_COUNT=1, a held org lock must not take the connection the
// provisioning needs to read and write the org's credentials.
func TestOrgSchedulerCredentialRepo_LockOrgCredentials_HolderCanUseCappedPool(t *testing.T) {
	pool := cappedPool(t, 1)
	repo := NewOrgSchedulerCredentialRepo(pool)
	ouID := "org-" + uuid.NewString()

	release, err := newOrgCredentialLocker(t, pool).LockOrgCredentials(context.Background(), ouID)
	require.NoError(t, err)
	defer release()

	// GetByOrgName takes no context, so bound it from outside. Not found is
	// fine: the point is that the query reaches the database at all.
	result := make(chan error, 1)
	go func() {
		_, err := repo.GetByOrgName(ouID)
		result <- err
	}()
	select {
	case err := <-result:
		require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		release() // frees the capped pool's only connection so the goroutine can finish
		t.Fatal("the holder must still reach the database through a capped pool")
	}
}
