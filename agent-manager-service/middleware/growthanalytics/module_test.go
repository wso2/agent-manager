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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrackable_ReportsMarkedRequest(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	fs, _ := withFakeSender(t)

	h := Trackable(func(w http.ResponseWriter, r *http.Request) {
		Mark(r.Context(), "amp.discovery.test", map[string]any{"action": "first", "kind": "a"})
		Mark(r.Context(), "ignored", map[string]any{"action": "second"})
		w.WriteHeader(http.StatusCreated)
	})
	h(httptest.NewRecorder(), authedRequest(http.MethodPost, "/m/x/things"))

	evt := waitForEvent(t, fs)
	if evt.Metadata["growth_action"] != "amp.discovery.test" || evt.Metadata["action"] != "second" || evt.Metadata["kind"] != "a" {
		t.Errorf("metadata = %v, want the first feature with merged props", evt.Metadata)
	}
	if evt.Response.Status != http.StatusCreated {
		t.Errorf("status = %d, want %d", evt.Response.Status, http.StatusCreated)
	}
}

func TestTrackable_SendsNothingUnmarked(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	fs, tokens := withFakeSender(t)

	Trackable(func(w http.ResponseWriter, _ *http.Request) {})(httptest.NewRecorder(), authedRequest(http.MethodPost, "/m/x/things"))
	Mark(t.Context(), "outside", nil)

	if len(*tokens) != 0 || len(fs.sent) != 0 {
		t.Errorf("sent an event for a request nothing marked")
	}
}
