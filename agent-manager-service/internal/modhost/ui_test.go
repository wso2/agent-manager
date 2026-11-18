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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

func TestUIListsActiveModulesWithResolvedPermissions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newWorld(t)
	mf := manifest("t-ui", nil, nil)
	mf.Permissions = []modkit.Permission{{Name: "view", Description: "View"}}
	mf.UI = &modkit.UI{
		Pages: []modkit.Page{
			{ID: "widgets/list", Permission: "view"},
			{ID: "widgets/agents", Permission: "agent:read"},
			{ID: "widgets/open"},
			{ID: "widgets/bad", Permission: "nosuch:perm"},
		},
		Nav: []modkit.NavItem{{Label: "Widgets", Page: "list", Permission: "view"}},
	}
	withUI := &testModule{manifest: mf}
	h, _ := replica(t, ctx, databases(t)[0], append(w.modules(), withUI))

	assert.Empty(t, h.UI(), "nothing with UI is active yet")
	_, err := h.Install(ctx, "t-ui", false)
	require.NoError(t, err)

	got := h.UI()
	require.Len(t, got, 1)
	assert.Equal(t, "t-ui", got[0].Name)
	assert.Equal(t, []modhost.UIEntry{
		{ID: "widgets/list", Permission: "t-ui:view"},
		{ID: "widgets/agents", Permission: "agent:read"},
		{ID: "widgets/open"},
	}, got[0].Pages)
	assert.Equal(t, []modhost.UIEntry{{ID: "list", Label: "Widgets", Permission: "t-ui:view"}}, got[0].Nav)

	require.NoError(t, h.Disable(ctx, "t-ui"))
	assert.Empty(t, h.UI())
}
