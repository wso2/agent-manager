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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func newBuildCredOrg(t *testing.T) string {
	t.Helper()
	ouID := "ou-build-" + uuid.NewString()
	t.Cleanup(func() {
		_ = db.GetDB().Where("ou_id = ?", ouID).Delete(&models.OrgBuildCredential{}).Error
	})
	return ouID
}

func TestOrgBuildCredentialRepository_UpsertAndGet(t *testing.T) {
	repo := NewOrgBuildCredentialRepo(db.GetDB())
	ouID := newBuildCredOrg(t)

	_, err := repo.GetByOrgName(ouID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	require.NoError(t, repo.Upsert(&models.OrgBuildCredential{
		OUID: ouID, OrgUUID: ouID, ClientID: "amp-build-" + ouID, AppID: "app-1", SecretRefName: "ref-1",
	}))
	got, err := repo.GetByOrgName(ouID)
	require.NoError(t, err)
	assert.Equal(t, "amp-build-"+ouID, got.ClientID)
	assert.Equal(t, "app-1", got.AppID)
	assert.Equal(t, "ref-1", got.SecretRefName)

	// A second upsert for the same org replaces the row rather than adding one.
	require.NoError(t, repo.Upsert(&models.OrgBuildCredential{
		OUID: ouID, OrgUUID: ouID, ClientID: "amp-build-" + ouID, AppID: "app-2", SecretRefName: "ref-2",
	}))
	got, err = repo.GetByOrgName(ouID)
	require.NoError(t, err)
	assert.Equal(t, "ref-2", got.SecretRefName)
	assert.Equal(t, "app-2", got.AppID, "a re-provisioned client replaces the stored application ID")
	var count int64
	require.NoError(t, db.GetDB().Model(&models.OrgBuildCredential{}).Where("ou_id = ?", ouID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

// The provisioning lock serializes one organization across connections (and so
// across replicas), and does not hold up another organization.
func TestOrgBuildCredentialRepository_ProvisioningLock(t *testing.T) {
	repo := NewOrgBuildCredentialRepo(db.GetDB())
	ctx := context.Background()
	ouA, ouB := newBuildCredOrg(t), newBuildCredOrg(t)

	holding := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- repo.WithProvisioningLock(ctx, ouA, func() error {
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding

	// Another organization is not blocked by ouA's lock.
	otherDone := make(chan error, 1)
	go func() { otherDone <- repo.WithProvisioningLock(ctx, ouB, func() error { return nil }) }()
	select {
	case err := <-otherDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a different organization waited on ouA's provisioning lock")
	}

	// The same organization waits until the holder finishes.
	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- repo.WithProvisioningLock(ctx, ouA, func() error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("a second holder entered while ouA's lock was held")
	case <-time.After(500 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-firstDone)
	select {
	case <-secondEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("the second holder never acquired ouA's lock after it was released")
	}
	require.NoError(t, <-secondDone)
}
