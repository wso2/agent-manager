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

package bedrock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Error codes AWS returns when the request is not authenticated (as opposed
// to authenticated but not allowed). See the CommonErrors pages of each
// service and the STS AssumeRole errors (evidence §2.13).
var authErrorCodes = map[string]bool{
	"UnrecognizedClientException": true,
	"InvalidClientTokenId":        true,
	"InvalidSignatureException":   true,
	"SignatureDoesNotMatch":       true,
	"IncompleteSignature":         true,
	"MissingAuthenticationToken":  true,
	"ExpiredTokenException":       true,
	"ExpiredToken":                true,
	"NotAuthorized":               true,
	"AuthFailure":                 true,
}

// Throttling codes: the modelled ThrottlingException is 429 on restJson
// services, while the common-errors pages give 400 (evidence §2.13).
var throttleErrorCodes = map[string]bool{
	"ThrottlingException":      true,
	"Throttling":               true,
	"TooManyRequestsException": true,
	"RequestLimitExceeded":     true,
	"ThrottledException":       true,
}

var urlRE = regexp.MustCompile(`https?://[^\s"']+`)

// truncatedError marks a listing that stopped at the page bound. It keeps
// the call's outcome "ok" and says the results are partial.
func truncatedError(pages int) error {
	return &sa.CallError{Outcome: models.ShadowCallOK, Message: fmt.Sprintf("stopped after %d pages; results are partial", pages)}
}

// awsError classifies an SDK error for the scan result. The message keeps
// the AWS error code and its message, scrubbed; URLs are reduced to their
// origin so a pagination token in a query string is never stored.
func awsError(err error) error {
	if err == nil {
		return nil
	}
	var ce *sa.CallError
	if errors.As(err, &ce) {
		return ce
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &sa.CallError{Outcome: models.ShadowCallFailed, Message: cleanMessage("cancelled: " + err.Error())}
	}
	if credErr := credentialsError(err); credErr != nil {
		msg := "could not get AWS credentials (check the access key, and that the role can be assumed)"
		var apiErr smithy.APIError
		if errors.As(credErr, &apiErr) {
			msg += ": " + apiErr.ErrorCode()
		}
		return &sa.CallError{Outcome: models.ShadowCallAuthFailed, Message: msg}
	}
	status := 0
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) {
		status = re.HTTPStatusCode()
	}
	var de *smithy.DeserializationError
	if errors.As(err, &de) {
		return sa.Unsupported("the response did not match the documented shape")
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		outcome := sa.OutcomeForStatus(status)
		switch {
		case authErrorCodes[code]:
			outcome = models.ShadowCallAuthFailed
		case throttleErrorCodes[code]:
			outcome = models.ShadowCallRateLimited
		case code == "AccessDeniedException" || code == "AccessDenied":
			outcome = models.ShadowCallPermissionDenied
		case code == "ResourceNotFoundException":
			outcome = models.ShadowCallNotFound
		case status == 0:
			outcome = models.ShadowCallFailed
		}
		msg := code
		if m := apiErr.ErrorMessage(); m != "" && m != code {
			msg += ": " + m
		}
		return &sa.CallError{Outcome: outcome, Status: status, Message: cleanMessage(msg)}
	}
	if status != 0 {
		return &sa.CallError{Outcome: sa.OutcomeForStatus(status), Status: status, Message: http.StatusText(status)}
	}
	return &sa.CallError{Outcome: models.ShadowCallFailed, Message: cleanMessage(err.Error())}
}

// cleanMessage reduces URLs to their origin and scrubs credential-like text.
func cleanMessage(s string) string {
	return sa.ScrubText(urlRE.ReplaceAllStringFunc(s, sa.Origin))
}

// credentialsError returns the part of err that comes from getting the
// request's credentials: a signing failure, or the STS AssumeRole call the
// credentials provider made (wrapped as "get identity: ..." by the SDK).
func credentialsError(err error) error {
	var se *v4.SigningError
	if errors.As(err, &se) {
		return se
	}
	// The outer OperationError is the called service's; the credentials
	// provider's STS call is wrapped further down the chain.
	for e := err; e != nil; {
		var oe *smithy.OperationError
		if !errors.As(e, &oe) {
			return nil
		}
		if oe.Service() == "STS" {
			return oe
		}
		e = oe.Unwrap()
	}
	return nil
}

func isAuthFailure(err error) bool {
	return err != nil && sa.OutcomeOf(err) == models.ShadowCallAuthFailed
}
