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

package controllers

import (
	"fmt"
	"net/http"
	"regexp"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxPromotionReasonLen keeps the rendered "message: reason" pair inside the
// legibility budget blocked promotions already follow.
const maxPromotionReasonLen = 120

var (
	// A manager supplies the message, so internal addresses and token shaped
	// strings are removed before anything reaches the caller.
	promotionURLPattern   = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://\S+`)
	promotionTokenPattern = regexp.MustCompile(`[A-Za-z0-9_\-.]{32,}`)
)

// writeIdentityPromotionError answers a typed identity promotion refusal. The
// body carries the error's own message (scrubbed) and how many values are
// missing, never their names or its cause, which is logged by the service.
func writeIdentityPromotionError(w http.ResponseWriter, err *services.IdentityPromotionError) {
	reason := scrubPromotionText(err.Message)
	switch err.Kind {
	case services.IdentityPromotionUnavailable:
		utils.WriteErrorResponseWithReason(w, http.StatusServiceUnavailable,
			"Identity service unavailable, the agent was not promoted", reason, utils.ErrCodeServiceUnavailable)
	case services.IdentityPromotionForbidden:
		utils.WriteErrorResponseWithReason(w, http.StatusForbidden,
			"Identity promotion is not permitted", reason, utils.ErrCodeForbidden)
	case services.IdentityPromotionConflict:
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Identity promotion conflicts with the target environment", reason, utils.ErrCodeConflict)
	case services.IdentityPromotionMissingValues:
		utils.WriteValidationErrorResponse(w, utils.NewInvalidInputError(
			"Promotion blocked: the target environment lacks required values", missingValuesReason(len(err.MissingValues)),
		))
	case services.IdentityPromotionBlocked:
		utils.WriteValidationErrorResponse(w, utils.NewInvalidInputError("Promotion blocked", reason))
	default:
		utils.WriteErrorResponseWithReason(w, http.StatusInternalServerError,
			"Failed to promote agent", "Internal server error", utils.ErrCodeInternalError)
	}
}

func scrubPromotionText(text string) string {
	text = promotionURLPattern.ReplaceAllString(text, "[address removed]")
	text = promotionTokenPattern.ReplaceAllString(text, "[value removed]")
	text = utils.SanitizeForLog(text)
	if utf8.RuneCountInString(text) > maxPromotionReasonLen {
		runes := []rune(text)
		text = string(runes[:maxPromotionReasonLen-3]) + "..."
	}
	return text
}

// missingValuesReason counts the missing values. Their names are never sent: a
// name can reveal how the target environment is configured.
func missingValuesReason(count int) string {
	if count <= 0 {
		return "required values are missing"
	}
	return fmt.Sprintf("%d required value(s) are missing in the target environment", count)
}
