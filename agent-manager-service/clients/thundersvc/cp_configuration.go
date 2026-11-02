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

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxCPResponseBytes bounds a CP response.
const maxCPResponseBytes int64 = 1 << 20

const (
	cpApplyPollInterval = 250 * time.Millisecond
	cpActivationTimeout = 60 * time.Second
)

// CPRequestError is a CP management request the CP answered with a non success
// status. It carries the status only, never the response body.
type CPRequestError struct{ Status int }

func (e *CPRequestError) Error() string {
	return fmt.Sprintf("CP management request returned HTTP %d", e.Status)
}

// CPConfigurationClient makes a management change on the control plane (CP)
// visible on the environment gateway. A CP write is only captured into a
// configuration version and applied to the gateway afterwards.
type CPConfigurationClient struct {
	httpClient *http.Client
	authorizer ManagementRequestAuthorizer
}

func NewCPConfigurationClient(authorizer ManagementRequestAuthorizer) (*CPConfigurationClient, error) {
	if authorizer == nil {
		return nil, errors.New("CP management authorizer is required")
	}
	return &CPConfigurationClient{httpClient: newCPHTTPClient(http.DefaultTransport), authorizer: authorizer}, nil
}

// newCPHTTPClient never follows redirects, so a credential is not replayed to
// another destination.
func newCPHTTPClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Timeout:       30 * time.Second,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Activate captures the CP configuration as it now stands, applies it to the
// target gateway and confirms the gateway reports that version.
//
// The apply names no resource selection, so the gateway keeps its standing
// choice, and it never confirms removals: an apply that would remove many
// resources is refused by the CP and surfaces here as an error. Activation is
// idempotent and safe to retry.
func (c *CPConfigurationClient) Activate(ctx context.Context, target EnvironmentIdentityTarget, note string) error {
	ctx, cancel := context.WithTimeout(ctx, cpActivationTimeout)
	defer cancel()
	if _, err := c.capture(ctx, target, note); err != nil {
		return fmt.Errorf("capture CP configuration: %w", err)
	}
	// Named versions can roll a gateway back when another replica captures and
	// applies a newer version. Apply the latest and verify it is still current.
	for {
		result, err := c.apply(ctx, target, "latest")
		if err != nil {
			return fmt.Errorf("apply CP configuration to gateway: %w", err)
		}
		if result.Queued {
			if result.QueuedVersion == "" {
				return errors.New("CP gateway queued an apply without a version")
			}
			if err := c.waitForAppliedVersion(ctx, target, result.QueuedVersion); err != nil {
				return fmt.Errorf("wait for queued CP gateway apply: %w", err)
			}
			// The queued apply has no import report in its 202 response. Apply
			// again so a refused resource cannot be mistaken for success.
			continue
		}
		applied, err := c.appliedVersion(ctx, target)
		if err != nil {
			return fmt.Errorf("verify CP gateway apply: %w", err)
		}
		latest, err := c.latestVersion(ctx, target)
		if err != nil {
			return fmt.Errorf("verify latest CP configuration: %w", err)
		}
		if applied == latest {
			return nil
		}
		if err := waitForCPPoll(ctx); err != nil {
			return fmt.Errorf("CP gateway did not report the latest configuration version: %w", err)
		}
	}
}

func (c *CPConfigurationClient) waitForAppliedVersion(ctx context.Context, target EnvironmentIdentityTarget, queued string) error {
	for {
		applied, err := c.appliedVersion(ctx, target)
		if err != nil {
			return err
		}
		if applied == queued {
			return nil
		}
		latest, err := c.latestVersion(ctx, target)
		if err != nil {
			return err
		}
		if applied == latest {
			return nil
		}
		if err := waitForCPPoll(ctx); err != nil {
			return err
		}
	}
}

func waitForCPPoll(ctx context.Context) error {
	timer := time.NewTimer(cpApplyPollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// capture records the configuration as the current version, as the CP's own
// change capture does, and returns its hash.
func (c *CPConfigurationClient) capture(ctx context.Context, target EnvironmentIdentityTarget, note string) (string, error) {
	var response struct {
		Version string            `json:"version"`
		Skipped []json.RawMessage `json:"skipped"`
	}
	payload := map[string]any{"current": true, "note": note}
	if err := c.request(ctx, target, http.MethodPost, "/configuration-versions", payload, &response); err != nil {
		return "", err
	}
	if response.Version == "" {
		return "", errors.New("CP configuration capture returned no version")
	}
	if len(response.Skipped) > 0 {
		return "", fmt.Errorf("CP configuration capture skipped %d resource(s)", len(response.Skipped))
	}
	return response.Version, nil
}

type cpApplyResult struct {
	Missing       json.RawMessage `json:"missing"`
	Import        json.RawMessage `json:"import"`
	Recorded      bool            `json:"recorded"`
	Queued        bool            `json:"queued"`
	QueuedVersion string          `json:"queuedVersion"`
}

func (c *CPConfigurationClient) apply(ctx context.Context, target EnvironmentIdentityTarget, version string) (cpApplyResult, error) {
	var response cpApplyResult
	path := "/gateways/" + url.PathEscape(target.GatewayID) + "/apply"
	if err := c.request(ctx, target, http.MethodPost, path, map[string]any{"version": version}, &response); err != nil {
		return response, err
	}
	if m := strings.TrimSpace(string(response.Missing)); m != "" && m != "null" && m != "{}" && m != "[]" {
		return response, errors.New("CP gateway apply reported missing environment values")
	}
	if response.Queued {
		return response, nil
	}
	if !response.Recorded {
		return response, errors.New("CP gateway apply did not record the version")
	}
	var report struct {
		Summary *struct {
			Failed int `json:"failed"`
		} `json:"summary"`
		Results []struct {
			ResourceType string `json:"resourceType"`
			ResourceID   string `json:"resourceId"`
			Status       string `json:"status"`
		} `json:"results"`
	}
	if len(response.Import) == 0 || json.Unmarshal(response.Import, &report) != nil || report.Summary == nil {
		return response, errors.New("CP gateway apply returned no valid import report")
	}
	var refused []string
	failedResults := 0
	for _, item := range report.Results {
		if item.Status == "failed" {
			failedResults++
			if len(refused) < 5 {
				refused = append(refused, strings.TrimSpace(item.ResourceType+" "+item.ResourceID))
			}
		}
	}
	if report.Summary.Failed > 0 || failedResults > 0 {
		failed := max(report.Summary.Failed, failedResults)
		if len(refused) > 0 {
			return response, fmt.Errorf("CP gateway refused %d resource(s): %s", failed, strings.Join(refused, ", "))
		}
		return response, fmt.Errorf("CP gateway refused %d resource(s)", failed)
	}
	return response, nil
}

func (c *CPConfigurationClient) latestVersion(ctx context.Context, target EnvironmentIdentityTarget) (string, error) {
	var response []struct {
		Version string `json:"version"`
	}
	if err := c.request(ctx, target, http.MethodGet, "/configuration-versions", nil, &response); err != nil {
		return "", err
	}
	if len(response) == 0 || response[0].Version == "" {
		return "", errors.New("CP returned no latest configuration version")
	}
	return response[0].Version, nil
}

func (c *CPConfigurationClient) appliedVersion(ctx context.Context, target EnvironmentIdentityTarget) (string, error) {
	var response struct {
		GatewayID      string `json:"gatewayId"`
		AppliedVersion string `json:"appliedVersion"`
	}
	path := "/gateways/" + url.PathEscape(target.GatewayID) + "/applied-version"
	if err := c.request(ctx, target, http.MethodGet, path, nil, &response); err != nil {
		return "", err
	}
	if response.GatewayID != target.GatewayID {
		return "", errors.New("CP applied-version response identifies another gateway")
	}
	return response.AppliedVersion, nil
}

func (c *CPConfigurationClient) request(ctx context.Context, target EnvironmentIdentityTarget,
	method, path string, payload, result any,
) error {
	return c.requestLimited(ctx, target, method, path, payload, result, maxCPResponseBytes)
}

// requestLimited is request with a caller chosen response size limit. A response
// over the limit is an error, never a truncated decode.
func (c *CPConfigurationClient) requestLimited(ctx context.Context, target EnvironmentIdentityTarget,
	method, path string, payload, result any, maxBytes int64,
) error {
	if err := target.validateManagement(); err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode CP request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(target.ManagementURL, "/")+path, body)
	if err != nil {
		return fmt.Errorf("build CP request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := authorizeCPRequest(ctx, c.authorizer, target, req); err != nil {
		return err
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		// A transport error carries the request URL, which holds the gateway and, for
		// a diff, the configuration version. Only the cause is kept.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("CP management request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &CPRequestError{Status: response.StatusCode}
	}
	if result == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("read CP management response: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return errors.New("CP management response is larger than the allowed size")
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("decode CP management response: %w", err)
	}
	return nil
}

// authorizeCPRequest scopes req to the target deployment and lets the injected
// authorizer add credentials. The authorizer may not redirect the request or
// change its deployment scope, and must supply a credential.
func authorizeCPRequest(ctx context.Context, authorizer ManagementRequestAuthorizer,
	target EnvironmentIdentityTarget, req *http.Request,
) error {
	method, requestURL := req.Method, req.URL.String()
	req.Header.Del("Authorization")
	req.Header.Set("X-Deployment-Id", target.DeploymentID)
	if err := authorizer.Authorize(ctx, target, req); err != nil {
		return fmt.Errorf("authorize CP request: %w", err)
	}
	if req.URL.String() != requestURL || req.Method != method {
		return errors.New("CP management authorizer changed the request destination or method")
	}
	if req.Header.Get("Authorization") == "" {
		return errors.New("CP management authorizer provided no authorization")
	}
	if req.Header.Get("X-Deployment-Id") != target.DeploymentID {
		return errors.New("CP management authorizer changed the deployment scope")
	}
	return nil
}
