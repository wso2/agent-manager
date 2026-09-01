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

package thundersvc

import "context"

//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/thunder_prober_fake.go . Prober:ThunderProberMock

// Prober checks whether an env-Thunder instance is reachable. It exists as an
// interface (rather than callers invoking ThunderProbe directly) so it can be
// injected and mocked in unit tests.
type Prober interface {
	// Probe reports reachability of the env-Thunder instance addressed by
	// thunderURL (its registered EnvThunderURL.ThunderURL — see
	// ThunderExternalTokenURL). thunderURL is required: callers only ever
	// probe an environment that already has one registered — there is no
	// address to probe otherwise, and no fallback pattern computed from org/env.
	// callerSupplied reports whether thunderURL was supplied directly by the
	// caller (no handle) rather than computed by AMS from a handle — see
	// thunderURLCandidate's doc comment for why this decides which candidate
	// gets SSRF-hardened.
	Probe(ctx context.Context, org, env, thunderURL string, callerSupplied bool) bool
}

// liveProber is the production Prober, backed by ThunderProbe.
type liveProber struct{}

// NewProber returns the production Prober.
func NewProber() Prober {
	return liveProber{}
}

func (liveProber) Probe(ctx context.Context, org, env, thunderURL string, callerSupplied bool) bool {
	return ThunderProbe(ctx, org, env, thunderURL, callerSupplied)
}
