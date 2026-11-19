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

package reconcile

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// heldPlacementKind holds every environment's placement until released; the fleet-wide pass finds nothing.
type heldPlacementKind struct {
	stubKind
	entered chan uuid.UUID
	release chan struct{}
}

func (k heldPlacementKind) UnplacedRecords(_ context.Context, _ string, env *uuid.UUID, _ string, _ int) ([]Record, error) {
	if env == nil {
		return nil, nil
	}
	k.entered <- *env
	<-k.release
	return nil, nil
}

func (heldPlacementKind) Targets(context.Context, Record) (uuid.UUID, bool, error) {
	return uuid.Nil, false, nil
}

func (heldPlacementKind) PlacedOn(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

// A requested placement runs on the worker, and Stop waits for one in progress.
func TestStopWaitsForARequestedPlacement(t *testing.T) {
	kind := heldPlacementKind{entered: make(chan uuid.UUID, 1), release: make(chan struct{})}
	core := NewCore(nil, nil, nil, Kinds{kind}, Config{
		RenderInterval: time.Hour, PlaceInterval: time.Hour,
		DriftInterval: time.Hour,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker := NewWorker(core, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, worker.Start(context.Background()))
	env := uuid.New()

	worker.RequestPlaceEnv("ou-1", env)
	select {
	case placing := <-kind.entered:
		assert.Equal(t, env, placing)
	case <-time.After(5 * time.Second):
		t.Fatal("the requested placement never ran")
	}
	stopped := make(chan struct{})
	go func() {
		_ = worker.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned while a placement was in progress")
	case <-time.After(100 * time.Millisecond):
	}
	close(kind.release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop never returned")
	}
}
