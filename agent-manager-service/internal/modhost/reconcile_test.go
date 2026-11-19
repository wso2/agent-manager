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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

func TestReconcileMatchesTheDeclaredList(t *testing.T) {
	for _, d := range databases(t) {
		t.Run(d.driver, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := newWorld(t)
			h, _ := replica(t, ctx, d, w.modules())

			// Listing a consumer installs its provider too.
			steps, err := h.Reconcile(ctx, []modhost.Desired{{Name: "t-consumer", Enabled: true}})
			require.NoError(t, err)
			assert.Equal(t, []modhost.Step{{Module: "t-store", Action: "install"}, {Module: "t-consumer", Action: "install"}}, steps)
			assert.Equal(t, []string{"t-builtin", "t-store", "t-consumer"}, h.Active())

			// Running it again changes nothing.
			steps, err = h.Reconcile(ctx, []modhost.Desired{{Name: "t-consumer", Enabled: true}})
			require.NoError(t, err)
			assert.Empty(t, steps)

			// An empty list disables what is not built in, consumers first.
			steps, err = h.Reconcile(ctx, nil)
			require.NoError(t, err)
			assert.Equal(t, []modhost.Step{{Module: "t-consumer", Action: "disable"}, {Module: "t-store", Action: "disable"}}, steps)
			assert.Equal(t, []string{"t-builtin"}, h.Active())

			// Listed disabled: the store stays installed but off; enabled again on request.
			_, err = h.Reconcile(ctx, []modhost.Desired{{Name: "t-store", Enabled: false}})
			require.NoError(t, err)
			assert.Equal(t, []string{"t-builtin"}, h.Active())
			steps, err = h.Reconcile(ctx, []modhost.Desired{{Name: "t-store", Enabled: true}})
			require.NoError(t, err)
			assert.Equal(t, []modhost.Step{{Module: "t-store", Action: "enable"}}, steps)

			// A provider that is not listed stays while a listed module needs it.
			_, err = h.Reconcile(ctx, []modhost.Desired{{Name: "t-consumer", Enabled: true}})
			require.NoError(t, err)
			assert.Contains(t, h.Active(), "t-store")

			// A provider listed disabled under an active consumer is refused.
			_, err = h.Reconcile(ctx, []modhost.Desired{{Name: "t-consumer", Enabled: true}, {Name: "t-store", Enabled: false}})
			assert.True(t, modhost.IsResolveError(err), "got %v", err)

			for _, bad := range [][]modhost.Desired{
				{{Name: "t-builtin", Enabled: false}},
				{{Name: "nosuch", Enabled: true}},
			} {
				_, err := h.Reconcile(ctx, bad)
				assert.True(t, modhost.IsResolveError(err), "%v: got %v", bad, err)
			}
		})
	}
}

func TestReconcileLeavesAListedDisabledModuleAlone(t *testing.T) {
	for _, d := range databases(t) {
		t.Run(d.driver, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := newWorld(t)
			var activations atomic.Int32
			provide := w.store.activate
			w.store.activate = func(rt modkit.Runtime) error {
				activations.Add(1)
				return provide(rt)
			}
			h, g := replica(t, ctx, d, w.modules())
			list := []modhost.Desired{{Name: "t-store", Enabled: false}}

			// First boot: never installed, so installed once and then disabled.
			steps, err := h.Reconcile(ctx, list)
			require.NoError(t, err)
			assert.Equal(t, []modhost.Step{{Module: "t-store", Action: "install"}, {Module: "t-store", Action: "disable"}}, steps)
			assert.NotContains(t, h.Active(), "t-store")

			// Every later boot changes nothing: no install, no activation, no
			// new generation for the other replicas to follow.
			before := activations.Load()
			gen := count(t, g, `SELECT value FROM module_generation WHERE id = 1`)
			for range 2 {
				steps, err = h.Reconcile(ctx, list)
				require.NoError(t, err)
				assert.Empty(t, steps)
			}
			assert.Equal(t, before, activations.Load())
			assert.Equal(t, gen, count(t, g, `SELECT value FROM module_generation WHERE id = 1`))
			assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM modules WHERE name = 't-store' AND state = 'disabled'`))
		})
	}
}

func TestReplicasReconcileTogether(t *testing.T) {
	for _, d := range databases(t) {
		t.Run(d.driver, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := newWorld(t)
			h1, g := replica(t, ctx, d, w.modules())
			h2, _ := replica(t, ctx, d, w.modules())

			for _, list := range [][]modhost.Desired{
				{{Name: "t-consumer", Enabled: true}},
				{{Name: "t-store", Enabled: true}},
				nil,
			} {
				var wg sync.WaitGroup
				errs := make([]error, 2)
				for i, h := range []*modhost.Host{h1, h2} {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, errs[i] = h.Reconcile(ctx, list)
					}()
				}
				wg.Wait()
				require.NoError(t, errs[0], "%v", list)
				require.NoError(t, errs[1], "%v", list)
			}
			assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM modules WHERE state = 'active' AND name <> 't-builtin'`))
		})
	}
}
