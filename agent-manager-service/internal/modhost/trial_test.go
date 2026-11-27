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

package modhost

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// plainModule activates with nothing.
type plainModule struct{ name string }

func (m plainModule) Manifest() modkit.Manifest {
	return modkit.Manifest{Name: m.name, Version: "1.0.0", CoreAPI: modkit.CoreAPIVersion}
}
func (plainModule) Migrations() fs.FS                              { return nil }
func (plainModule) UI() fs.FS                                      { return nil }
func (plainModule) Activate(context.Context, modkit.Runtime) error { return nil }

// A target the replica leaves out for its store fails the trial, though the
// trial never activates it.
func TestTrialRefusesATargetLeftOut(t *testing.T) {
	h := NewWith(nil, "sqlite", slog.New(slog.DiscardHandler), []modkit.Module{plainModule{"t-a"}})
	next := map[string]string{"t-a": StateActive}

	_, err := h.trial(context.Background(), next, []string{"t-a"}, nil)
	require.NoError(t, err)

	_, err = h.trial(context.Background(), next, []string{"t-a"}, map[string]error{"t-a": errors.New("store down")})
	var re *ResolveError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "t-a failed to activate: store down", re.Msg)
}
