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

// Cross-organization gateway fleet health.
//
// Kept in its own file because every other method on PlatformGatewayService is
// tenant-scoped and takes an ouID, and this one is not: it reads every
// organization's rows. Anything added here inherits that, so it belongs to a
// platform-admin route and nothing else — see
// middleware.RequirePlatformAdminOU.

package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxFailedGatewayDetailRows caps the detail list. The number of failed
// gateways grows with the number of tenants and this endpoint is polled, so the
// list has to be bounded somewhere; what must not happen is returning a page
// while the response reads as the whole set, which is what Truncated is for.
const maxFailedGatewayDetailRows = 500

// FailedGatewayDetail names one failed gateway. OrganizationID is on every row
// because the caller is cross-org and cannot otherwise tell whose gateway this
// is.
type FailedGatewayDetail struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OrganizationID string    `json:"organizationId"`
	LastUpdatedAt  time.Time `json:"lastUpdatedAt"`
}

// GatewayFailureSummaryQuery is the input to one health sweep.
//
// The two thresholds are unrelated and easy to confuse: one decides whether a
// single gateway counts as failed, the other whether the fleet as a whole is
// unhealthy. Named fields rather than positional arguments so a call site says
// which is which. Both are passed in rather than read from config here, so the
// arithmetic is testable without the environment.
type GatewayFailureSummaryQuery struct {
	// StalenessThreshold is how long a gateway must have been disconnected
	// before it counts as failed — the near edge of the window.
	StalenessThreshold time.Duration
	// MaxAge is the far edge: a disconnected gateway whose last update is older
	// than this is treated as decommissioned rather than failing. Must exceed
	// StalenessThreshold.
	MaxAge time.Duration
	// FailurePercentageThreshold is the share of the fleet, in percent, at or
	// above which the fleet is unhealthy.
	FailurePercentageThreshold float64
	// IncludeDetails additionally fetches the failed gateways themselves.
	IncludeDetails bool
}

// GatewayFailureSummaryResponse is the platform-wide gateway health summary.
//
// Every threshold that went into the verdict is echoed back, because Failed and
// FailurePercentage are meaningless without them: the same fleet yields a
// different answer at a different threshold, and a caller graphing this needs to
// know which instant and which policy it describes.
type GatewayFailureSummaryResponse struct {
	// Total is every gateway in the deployment, abandoned ones included.
	Total int64 `json:"total"`
	// Abandoned is the disconnected gateways last updated before the window's
	// far edge. Excluded from Considered, and so from the percentage.
	Abandoned int64 `json:"abandoned"`
	// Considered is the denominator: Total minus Abandoned. The fleet the
	// verdict is actually about.
	Considered int64 `json:"considered"`
	// Failed is the gateways inside the failure window.
	Failed int64 `json:"failed"`

	// FailurePercentage is Failed as a percentage of Considered — NOT of Total,
	// so a pile of decommissioned gateways cannot dilute a real outage away.
	// Rounded to two decimal places. Zero when Considered is zero.
	FailurePercentage float64 `json:"failurePercentage"`
	// Healthy is the verdict: FailurePercentage is below
	// FailurePercentageThreshold AND the fleet has not gone silent (see the
	// no-signal case in GetCrossOrgGatewayFailureSummary). It is therefore NOT
	// derivable from FailurePercentage alone — read this field, do not
	// recompute it.
	//
	// Always present, and it is the field to branch on. The summary form also
	// signals this through the HTTP status, but a detailed request answers 200
	// regardless, so the status is not a reliable channel for every caller —
	// this is.
	Healthy bool `json:"healthy"`

	// FailurePercentageThreshold is the percentage at or above which Healthy is
	// false.
	FailurePercentageThreshold float64 `json:"failurePercentageThreshold"`
	// StalenessThresholdSeconds is how long a gateway must have been
	// disconnected to be counted in Failed.
	StalenessThresholdSeconds int `json:"stalenessThresholdSeconds"`
	// MaxAgeSeconds is how stale a disconnected gateway may be and still count.
	// Anything last updated before it is excluded from Failed — but not from
	// Total, so the two can differ by more than Failed alone explains.
	MaxAgeSeconds int       `json:"maxAgeSeconds"`
	EvaluatedAt   time.Time `json:"evaluatedAt"`

	// FailedGateways is populated only when details were requested. Omitted
	// rather than sent empty, so an absent list is distinguishable from a fleet
	// with nothing wrong with it.
	FailedGateways []FailedGatewayDetail `json:"failedGateways,omitempty"`
	// Truncated says FailedGateways was capped and is not the whole set. Failed
	// remains the authoritative count.
	Truncated bool `json:"truncated,omitempty"`
}

// GetCrossOrgGatewayFailureSummary counts gateways across every organization,
// how many have been disconnected past the staleness threshold, and whether that
// share puts the fleet over the failure-percentage threshold.
//
// A gateway is failed when it is not connected (is_active false) and its last
// liveness change falls inside the window: older than StalenessThreshold, and
// newer than MaxAge. The far edge matters as much as the near one — without it a
// decommissioned gateway is reported as broken forever.
//
// Age never removes a CONNECTED gateway from anything. updated_at is just as old
// on one that has held its connection for months, because nothing writes the
// column while a connection lives, so an age-only rule would drop the healthiest
// gateways in the fleet and inflate the percentage rather than sharpen it. Only
// a gateway that is both old and disconnected is abandoned.
//
// The percentage is taken over Considered — every gateway except the abandoned
// ones — so decommissioned rows cannot dilute a real outage. Total and Abandoned
// are both reported so the denominator is checkable rather than implied.
//
// "Every gateway abandoned" and "no gateways at all" are deliberately NOT the
// same verdict. A deployment with gateways, none of which are still in scope,
// is reported unhealthy: the fleet has gone silent, and the 0% it would
// otherwise show is an artefact of an empty denominator rather than a healthy
// fleet. A deployment with no gateways at all is healthy — nothing is failing
// when nothing exists.
//
// When IncludeDetails is set, the failed rows are fetched in a second query, so
// len(FailedGateways) can lag Failed by whatever changed in between. Failed is
// the number to trust; the list is a snapshot taken beside it, not derived from
// it.
func (s *PlatformGatewayService) GetCrossOrgGatewayFailureSummary(
	ctx context.Context, query GatewayFailureSummaryQuery,
) (*GatewayFailureSummaryResponse, error) {
	if query.StalenessThreshold <= 0 {
		return nil, fmt.Errorf("%w: gateway staleness threshold must be greater than zero", utils.ErrBadRequest)
	}
	// An upper bound at or below the lower one is an empty window: no updated_at
	// can satisfy both, so every fleet reports perfectly healthy. Refused rather
	// than served, because the answer would look valid and never be.
	if query.MaxAge <= query.StalenessThreshold {
		return nil, fmt.Errorf(
			"%w: gateway failure max age must be greater than the staleness threshold",
			utils.ErrBadRequest,
		)
	}
	// Guarded here as well as at startup: this decides whether callers are told
	// the fleet is broken, and a zero value arriving from an uninitialised
	// struct would silently report every fleet unhealthy.
	//
	// NaN needs its own test because a range check cannot catch it — every
	// comparison against NaN is false, so it reads as "in range" here and then
	// makes the `percentage < threshold` verdict below false for every fleet.
	if math.IsNaN(query.FailurePercentageThreshold) ||
		query.FailurePercentageThreshold <= 0 ||
		query.FailurePercentageThreshold > 100 {
		return nil, fmt.Errorf(
			"%w: gateway failure percentage threshold must be greater than zero and at most 100",
			utils.ErrBadRequest,
		)
	}

	evaluatedAt := time.Now()
	window := repositories.GatewayFailureWindow{
		StaleBefore:  evaluatedAt.Add(-query.StalenessThreshold),
		NotOlderThan: evaluatedAt.Add(-query.MaxAge),
	}

	counts, err := s.gatewayRepo.CountFailureSummaryAllOrgs(ctx, window)
	if err != nil {
		return nil, fmt.Errorf("failed to summarize gateway failures: %w", err)
	}

	// Abandoned gateways leave the denominator. Counting them would let a
	// deployment bury a real outage under decommissioned rows: 5 of 5 live
	// gateways down reads as 5% — comfortably healthy — once 95 forgotten ones
	// are in the divisor.
	considered := counts.Total - counts.Abandoned
	percentage := failurePercentage(considered, counts.Failed)

	// A fleet that exists but has nothing left to consider is not healthy, it is
	// silent: every gateway it has is disconnected and past MaxAge. The
	// percentage reads 0 there only because the denominator is empty, and
	// reporting that as healthy would switch the alarm off at the exact moment
	// the whole fleet has been down longest — a prolonged total outage would
	// cross MaxAge and turn the monitor green. An empty deployment (Total == 0)
	// is a different thing and stays healthy: nothing is failing when nothing
	// exists.
	noSignal := considered == 0 && counts.Total > 0

	resp := &GatewayFailureSummaryResponse{
		Total:      counts.Total,
		Abandoned:  counts.Abandoned,
		Considered: considered,
		Failed:     counts.Failed,

		FailurePercentage: percentage,
		// Compared against the same rounded value that is reported, not the raw
		// quotient. Rounding only the output would let a response read
		// "10% failing, healthy: true" against a threshold of 10 — the payload
		// contradicting its own verdict over a hidden fraction.
		Healthy: !noSignal && percentage < query.FailurePercentageThreshold,

		FailurePercentageThreshold: query.FailurePercentageThreshold,
		StalenessThresholdSeconds:  int(query.StalenessThreshold.Seconds()),
		MaxAgeSeconds:              int(query.MaxAge.Seconds()),
		EvaluatedAt:                evaluatedAt,

		FailedGateways: nil,
		Truncated:      false,
	}
	if !query.IncludeDetails {
		return resp, nil
	}

	// One more than the cap, so a full page is distinguishable from a page that
	// happens to end exactly at the cap.
	gateways, err := s.gatewayRepo.ListFailedGatewaysAllOrgs(ctx, window, maxFailedGatewayDetailRows+1)
	if err != nil {
		return nil, fmt.Errorf("failed to list failed gateways: %w", err)
	}

	if len(gateways) > maxFailedGatewayDetailRows {
		resp.Truncated = true
		slog.Warn("gateway failure summary detail list truncated",
			"cap", maxFailedGatewayDetailRows, "failed", counts.Failed)
		gateways = gateways[:maxFailedGatewayDetailRows]
	}

	resp.FailedGateways = make([]FailedGatewayDetail, 0, len(gateways))
	for _, gw := range gateways {
		if gw == nil {
			continue
		}
		resp.FailedGateways = append(resp.FailedGateways, FailedGatewayDetail{
			ID:             gw.UUID.String(),
			Name:           gw.Name,
			OrganizationID: gw.OUID,
			LastUpdatedAt:  gw.UpdatedAt,
		})
	}

	return resp, nil
}

// failurePercentage returns failed as a percentage of considered, to two decimal
// places, and 0 when there is nothing in scope.
//
// Rounded before it is compared to anything, so the number in the response and
// the number behind the verdict are the same number.
func failurePercentage(considered, failed int64) float64 {
	if considered <= 0 {
		return 0
	}
	return math.Round(float64(failed)/float64(considered)*10000) / 100
}
