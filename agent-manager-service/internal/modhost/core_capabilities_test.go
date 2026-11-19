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
package modhost_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
)

type fakeDirectory struct{ identityv1.Directory }

// A module that requires a core capability installs with nothing else present and finds the core's
// implementation; a module may not provide one.
func TestCoreCapabilities(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	found := make(chan identityv1.Directory, 1)
	user := &testModule{
		manifest: manifest("t-directory-user", []string{identityv1.ID}, nil),
		activate: func(rt modkit.Runtime) error {
			dir, ok := modkit.Lookup[identityv1.Directory](rt, identityv1.ID)
			if !ok {
				return fmt.Errorf("%s not resolvable", identityv1.ID)
			}
			select {
			case found <- dir:
			default:
			}
			return nil
		},
	}
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	h := modhost.NewWith(g, d.driver, quiet, []modkit.Module{user})
	h.PollInterval = 20 * time.Millisecond
	impl := fakeDirectory{}
	h.Attach(modhost.Deps{Capabilities: map[string]any{identityv1.ID: impl}})
	_, err := h.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, h.Boot(ctx))

	_, err = h.Install(ctx, "t-directory-user", false)
	require.NoError(t, err)
	assert.Equal(t, identityv1.Directory(impl), <-found)
	assert.Contains(t, h.Active(), "t-directory-user")

	assert.Panics(t, func() {
		modhost.NewWith(g, d.driver, quiet, []modkit.Module{&testModule{manifest: manifest("t-impostor", nil, []string{identityv1.ID})}})
	}, "a module may not provide a core capability")
	assert.Panics(t, func() {
		h.Attach(modhost.Deps{Capabilities: map[string]any{"made.up/v1": impl}})
	}, "only declared core capabilities can be attached")
}
