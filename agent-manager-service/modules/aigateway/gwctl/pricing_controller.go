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

package gwctl

import (
	"net/http"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// PricingController serves the model price catalog (/orgs/{orgName}/pricing), read-only.
type PricingController interface {
	GetPricing(w http.ResponseWriter, r *http.Request)
}

type pricingController struct {
	pricing gwsvc.PricingService
}

// NewPricingController creates a PricingController.
func NewPricingController(pricing gwsvc.PricingService) PricingController {
	return &pricingController{pricing: pricing}
}

const (
	pricingMaxSearch   = 255
	pricingMaxProvider = 64
)

// GetPricing handles GET /orgs/{orgName}/pricing: a page of the catalog by model name. The
// catalog is the same for every organization.
func (c *pricingController) GetPricing(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	q := gwsvc.PricingQuery{Search: query.Get("search"), Provider: query.Get("provider")}
	if len(q.Search) > pricingMaxSearch || len(q.Provider) > pricingMaxProvider {
		httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, "The search or provider is too long.", "", httpx.ErrCodeValidation)
		return
	}
	switch sort := query.Get("sort"); sort {
	case "", "model", "provider":
		q.Sort = sort
	default:
		httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, "sort must be model or provider.", "", httpx.ErrCodeValidation)
		return
	}
	for name, dst := range map[string]*int{"limit": &q.Limit, "offset": &q.Offset} {
		raw := query.Get(name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || (name == "limit" && (n < 1 || n > 200)) {
			msg := "limit must be a whole number from 1 to 200."
			if name == "offset" {
				msg = "offset must be a whole number of 0 or more."
			}
			httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, msg, "", httpx.ErrCodeValidation)
			return
		}
		*dst = n
	}
	resp, err := c.pricing.Catalog(r.Context(), q)
	if err != nil {
		writeBudgetError(w, r, err, "Failed to read model prices")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}
