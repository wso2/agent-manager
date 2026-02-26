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

package utils

import "time"

const (
	// RawThreshold is the maximum number of data points below which
	// trace-level aggregation is used instead of time-bucket aggregation.
	RawThreshold int64 = 50
)

// CalculateAdaptiveGranularity selects time-series granularity based on both
// the time range duration and the actual number of data points.
//
// If count <= RawThreshold, returns "trace" (group by trace_id for maximum
// granularity, ideal for users who are just starting out).
//
// Otherwise, picks a time-bucket granularity based on the duration:
//
//	<= 6 hours → "minute"
//	<= 7 days  → "hour"
//	<= 28 days → "day"
//	> 28 days  → "week"
func CalculateAdaptiveGranularity(d time.Duration, count int64) string {
	if count <= RawThreshold {
		return "trace"
	}

	switch {
	case d <= 6*time.Hour:
		return "minute"
	case d <= 7*24*time.Hour:
		return "hour"
	case d <= 28*24*time.Hour:
		return "day"
	default:
		return "week"
	}
}
