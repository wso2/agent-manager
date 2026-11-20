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

package growthanalytics

import (
	"context"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

const markKey ctxKey = overridesKey + 1

// mark is the feature a module route reported with Mark.
type mark struct {
	feature   string
	overrides *dimensionOverrides
}

// Trackable lets the handler of a module route report its request as feature
// usage with Mark; the event is sent after the response, as Track sends it.
// It returns handler unchanged when feature usage is not reported.
func Trackable(handler http.HandlerFunc) http.HandlerFunc {
	ga := config.GetConfig().GrowthAnalytics
	if !ga.Enabled || ga.MoesifCollectorBaseURL == "" {
		return handler
	}
	return func(w http.ResponseWriter, r *http.Request) {
		m := &mark{overrides: &dimensionOverrides{}}
		ctx := context.WithValue(r.Context(), markKey, m)
		holder := &statusHolder{code: http.StatusOK}
		sw := &statusRecordingWriter{ResponseWriter: w, holder: holder}
		reqSnapshot := snapshotRequest(r)
		userID := identifyUser(ctx)
		companyID := middleware.OUIDFromRequest(r)
		token := jwtassertion.GetJWTFromContext(ctx)

		handler(sw, r.WithContext(ctx))

		if m.feature != "" {
			reportEvent(r.Context(), ga, token, m.feature, nil, m.overrides, holder, reqSnapshot, userID, companyID)
		}
	}
}

// Mark reports the request a Trackable handler serves as usage of feature,
// with props as dimensions. A later call adds to props; its values win. It
// does nothing outside a Trackable request.
func Mark(ctx context.Context, feature string, props map[string]any) {
	m, ok := ctx.Value(markKey).(*mark)
	if !ok {
		return
	}
	if m.feature == "" {
		m.feature = feature
	}
	for k, v := range props {
		m.overrides.set(k, v)
	}
}
