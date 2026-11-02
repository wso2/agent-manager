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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const gatewayTestOUID = "ou-acme"

func gatewayFixture(name string) *models.Gateway {
	return &models.Gateway{
		UUID:        uuid.New(),
		Name:        name,
		DisplayName: name,
		OUID:        gatewayTestOUID,
		Vhost:       name + ".example.com",
	}
}

func TestGatewayIsManaged(t *testing.T) {
	assert.True(t, (&models.Gateway{Hosting: ""}).IsManaged())
	assert.True(t, (&models.Gateway{Hosting: models.GatewayHostingManaged}).IsManaged())
	assert.False(t, (&models.Gateway{Hosting: models.GatewayHostingSelfHosted}).IsManaged())
}

func TestGetGateway_EmitsHosting(t *testing.T) {
	cases := map[string]string{
		models.GatewayHostingSelfHosted: "SELF_HOSTED",
		models.GatewayHostingManaged:    "MANAGED",
		"":                              "MANAGED",
	}
	for stored, wire := range cases {
		t.Run("stored="+stored, func(t *testing.T) {
			gateway := gatewayFixture("edge")
			gateway.Hosting = stored
			repo := &repomocks.GatewayRepositoryMock{
				GetByUUIDFunc: func(string) (*models.Gateway, error) { return gateway, nil },
			}
			svc := NewPlatformGatewayService(repo, nil)

			resp, err := svc.GetGateway(gateway.UUID.String(), gatewayTestOUID)

			require.NoError(t, err)
			assert.Equal(t, wire, resp.Hosting)
		})
	}
}

// The spec advertises the path param as "Gateway UUID or name", and a name used to
// be rejected with a bare error that matched no case in handleGatewayErrors and
// surfaced as HTTP 500.
func TestGetGateway_ResolvesByName(t *testing.T) {
	gateway := gatewayFixture("edge")
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*models.Gateway, error) {
			assert.Equal(t, "edge", name)
			assert.Equal(t, gatewayTestOUID, ouID)
			return gateway, nil
		},
		// GetByUUIDFunc nil: a name must not be looked up as a UUID.
	}
	svc := NewPlatformGatewayService(repo, nil)

	resp, err := svc.GetGateway("edge", gatewayTestOUID)

	require.NoError(t, err)
	assert.Equal(t, gateway.UUID.String(), resp.ID)
	assert.Equal(t, "edge", resp.Name)
}

func TestGetGateway_ResolvesByUUID(t *testing.T) {
	gateway := gatewayFixture("edge")
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayID string) (*models.Gateway, error) {
			assert.Equal(t, gateway.UUID.String(), gatewayID)
			return gateway, nil
		},
		// GetByNameAndOrgIDFunc nil: a UUID must not fall through to a name lookup.
	}
	svc := NewPlatformGatewayService(repo, nil)

	resp, err := svc.GetGateway(gateway.UUID.String(), gatewayTestOUID)

	require.NoError(t, err)
	assert.Equal(t, gateway.UUID.String(), resp.ID)
}

func TestGetGateway_UnknownNameIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(_, _ string) (*models.Gateway, error) {
			return nil, utils.ErrGatewayNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway("ghost", gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

func TestGetGateway_UnknownUUIDIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway(uuid.New().String(), gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

// The name branch used to return the repository error verbatim, so a bare failure
// matched no case in handleGatewayErrors and surfaced as a 500 instead of a 404.
func TestGetGateway_UnknownNameFromGormIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(_, _ string) (*models.Gateway, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway("ghost", gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

// A nil gateway with no error reached GetGateway's OUID check and panicked.
func TestGetGateway_NilGatewayWithoutErrorIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(_, _ string) (*models.Gateway, error) {
			return nil, nil //nolint:nilnil // the shape under test
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway("ghost", gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

// A real repository failure must not be flattened into not-found.
func TestGetGateway_RepositoryErrorIsNotMaskedAsNotFound(t *testing.T) {
	boom := errors.New("connection refused")
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return nil, boom
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway(uuid.New().String(), gatewayTestOUID)

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, utils.ErrGatewayNotFound)
}

// Org isolation: a gateway UUID from another org reads as absent, not forbidden.
func TestGetGateway_OtherOrgGatewayIsNotFound(t *testing.T) {
	gateway := gatewayFixture("edge")
	gateway.OUID = "ou-other"
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return gateway, nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway(gateway.UUID.String(), gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

// fakeButValidShapedToken builds a token string that passes VerifyToken's
// format/UUID checks (36-byte UUID + "-" + random suffix) but was never
// issued — the exact shape an anonymous DoS attempt would submit.
func fakeButValidShapedToken() string {
	return uuid.New().String() + "-kQpL8vK9zXwR3tYbN7cF2mJ5hD1sA6e"
}

// Before the fix, GetActiveTokenByPrefix ran once per call with no negative
// caching: an anonymous caller submitting the same fake-but-valid-shaped key
// repeatedly triggered one real DB query every single time. This guards that
// a confirmed miss is remembered so a second lookup for the identical prefix
// is served from memory instead of hitting the repository again.
func TestVerifyToken_RepeatedFakeKey_DoesNotHitDBTwice(t *testing.T) {
	fakeToken := fakeButValidShapedToken()
	dbCalls := 0
	repo := &repomocks.GatewayRepositoryMock{
		GetActiveTokenByPrefixFunc: func(_ context.Context, _ string) (*models.GatewayToken, error) {
			dbCalls++
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err1 := svc.VerifyToken(context.Background(), fakeToken)
	_, err2 := svc.VerifyToken(context.Background(), fakeToken)

	assert.Error(t, err1)
	assert.Error(t, err2)
	assert.Equal(t, 1, dbCalls, "a repeated lookup for the same confirmed-absent prefix must be served from the negative cache, not the DB")
}

// Concurrent requests for the same missing prefix must be coalesced into a
// single repository call via singleflight, not one call per goroutine — the
// negative cache alone only protects *sequential* repeats, since concurrent
// callers can all observe IsKnownMiss==false before any of them has recorded
// the miss.
func TestVerifyToken_ConcurrentRequestsForSameMissingPrefix_SingleDBCall(t *testing.T) {
	fakeToken := fakeButValidShapedToken()
	var dbCalls atomic.Int32
	release := make(chan struct{})
	repo := &repomocks.GatewayRepositoryMock{
		GetActiveTokenByPrefixFunc: func(_ context.Context, _ string) (*models.GatewayToken, error) {
			dbCalls.Add(1)
			<-release // hold every call open until all goroutines have started
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.VerifyToken(context.Background(), fakeToken)
			assert.Error(t, err)
		}()
	}

	// Give every goroutine a chance to reach the singleflight-coalesced call
	// before releasing it, so a naive (non-serialized) implementation would
	// have already dispatched its own separate DB calls by now.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int32(1), dbCalls.Load(), "concurrent requests for one missing prefix must produce only one repository call")
}

// A cancelled context must reach the repository call so an in-flight DB
// lookup can actually be aborted — the fix threads ctx through VerifyToken
// instead of the DB call running to completion regardless of the caller
// having already given up.
func TestVerifyToken_PropagatesContextToRepository(t *testing.T) {
	fakeToken := fakeButValidShapedToken()
	var receivedCtx context.Context
	repo := &repomocks.GatewayRepositoryMock{
		GetActiveTokenByPrefixFunc: func(ctx context.Context, _ string) (*models.GatewayToken, error) {
			receivedCtx = ctx
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.VerifyToken(ctx, fakeToken)

	require.Error(t, err)
	require.NotNil(t, receivedCtx, "GetActiveTokenByPrefix must be reached with the caller's context, not a detached one")
	assert.ErrorIs(t, receivedCtx.Err(), context.Canceled, "the repository must receive the caller's (cancelled) context, not context.Background()")
}
