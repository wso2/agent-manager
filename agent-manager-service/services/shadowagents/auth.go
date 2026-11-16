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

package shadowagents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// awsRoleSessionName names the sessions Agent Manager opens in a customer
// account, so they are easy to find in the customer's CloudTrail.
const awsRoleSessionName = "wso2-agent-manager-shadow-discovery"

// AWSCredentials are the access key a connection holds and the role it
// assumes, if any.
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	RoleARN         string
	ExternalID      string
}

// AWSConfig builds the SDK configuration for one region. Credentials come
// only from the connection: the SDK never reads the host's environment,
// shared files or instance role, which belong to Agent Manager, not to the
// tenant. With a RoleARN, the access key is used only to assume the role
// through stsEndpoint.
func AWSConfig(httpClient *http.Client, region, stsEndpoint string, c AWSCredentials) (aws.Config, error) {
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return aws.Config{}, errors.New("an access key ID and secret access key are required")
	}
	cfg := aws.Config{
		Region:           region,
		Credentials:      aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, c.SessionToken)),
		HTTPClient:       httpClient,
		RetryMaxAttempts: 4,
		RetryMode:        aws.RetryModeStandard,
	}
	if c.RoleARN == "" {
		return cfg, nil
	}
	stsClient := sts.NewFromConfig(cfg, func(o *sts.Options) {
		if stsEndpoint != "" {
			o.BaseEndpoint = aws.String(stsEndpoint)
		}
	})
	provider := stscreds.NewAssumeRoleProvider(stsClient, c.RoleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = awsRoleSessionName
		if c.ExternalID != "" {
			o.ExternalID = aws.String(c.ExternalID)
		}
	})
	cfg.Credentials = aws.NewCredentialsCache(provider)
	return cfg, nil
}

// SigV4Authorizer signs plain HTTP requests with SigV4 for service in
// region, for AWS-hosted APIs without an SDK client.
func SigV4Authorizer(cfg aws.Config, service, region string) func(ctx context.Context, req *http.Request, body []byte) error {
	signer := v4.NewSigner()
	return func(ctx context.Context, req *http.Request, body []byte) error {
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return fmt.Errorf("retrieving AWS credentials: %w", err)
		}
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])
		req.Header.Set("X-Amz-Content-Sha256", hash)
		return signer.SignHTTP(ctx, creds, req, hash, service, region, time.Now())
	}
}

// AzureCredential returns an Entra client-secret credential that signs in
// at the connection's login endpoint. Instance discovery is off because the
// authority is the one the user configured.
func AzureCredential(httpClient *http.Client, loginEndpoint, tenantID, clientID, clientSecret string) (azcore.TokenCredential, error) {
	if tenantID == "" || clientID == "" || clientSecret == "" {
		return nil, errors.New("a tenant ID, client ID and client secret are required")
	}
	return azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, &azidentity.ClientSecretCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud:     cloud.Configuration{ActiveDirectoryAuthorityHost: strings.TrimRight(loginEndpoint, "/") + "/"},
			Transport: httpClient,
		},
		DisableInstanceDiscovery: true,
	})
}

// Azure token scopes.
const (
	AzureARMScope     = "https://management.azure.com/.default"
	AzureFoundryScope = "https://ai.azure.com/.default"
)

// AzureClient calls Azure REST APIs (ARM and the Foundry data plane)
// through the Azure SDK pipeline, which adds the bearer token and retries
// 429 and 5xx responses honouring Retry-After.
type AzureClient struct {
	pipeline runtime.Pipeline
}

// NewAzureClient returns a client whose requests carry a token for scope.
func NewAzureClient(httpClient *http.Client, cred azcore.TokenCredential, scope string) *AzureClient {
	pl := runtime.NewPipeline("wso2-agent-manager-shadow-discovery", "v1",
		runtime.PipelineOptions{PerRetry: []policy.Policy{runtime.NewBearerTokenPolicy(cred, []string{scope}, nil)}},
		&policy.ClientOptions{Transport: httpClient, Retry: policy.RetryOptions{
			MaxRetries: 3, MaxRetryDelay: maxRetryWait,
			// The SDK's defaults plus 529, which the Azure Monitor metrics
			// backend returns when throttling.
			StatusCodes: []int{
				http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError,
				http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529,
			},
		}},
	)
	return &AzureClient{pipeline: pl}
}

// GetJSON GETs u and decodes the JSON response into out.
func (c *AzureClient) GetJSON(ctx context.Context, u string, out any) error {
	req, err := runtime.NewRequest(ctx, http.MethodGet, u)
	if err != nil {
		return &CallError{Outcome: models.ShadowCallFailed, Message: "invalid endpoint URL"}
	}
	req.Raw().Header.Set("Accept", "application/json")
	resp, err := c.pipeline.Do(req)
	if err != nil {
		return azureError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromResponse(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxJSONBody+1))
	if err != nil {
		return &CallError{Outcome: models.ShadowCallFailed, Message: ScrubText("reading the response: " + err.Error())}
	}
	if len(body) > MaxJSONBody {
		return &CallError{Outcome: models.ShadowCallFailed, Message: "response larger than the allowed size"}
	}
	if err := jsonUnmarshal(body, out); err != nil {
		return Unsupported("the response did not match the documented shape: %s", jsonErrorKind(err))
	}
	return nil
}

func azureError(err error) error {
	var authErr *azidentity.AuthenticationFailedError
	if errors.As(err, &authErr) {
		status := 0
		if authErr.RawResponse != nil {
			status = authErr.RawResponse.StatusCode
		}
		return &CallError{Outcome: models.ShadowCallAuthFailed, Status: status, Message: "Microsoft Entra sign-in failed; check the tenant ID, client ID and client secret"}
	}
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return &CallError{Outcome: OutcomeForStatus(respErr.StatusCode), Status: respErr.StatusCode, Message: ScrubText(respErr.ErrorCode)}
	}
	return &CallError{Outcome: models.ShadowCallFailed, Message: ScrubText(err.Error())}
}

// GoogleScope is the OAuth scope Google requires for these reads; read-only
// access comes from the IAM roles granted to the service account (the
// read-only scope is rejected by reasoningEngines.list and timeSeries.list).
const GoogleScope = "https://www.googleapis.com/auth/cloud-platform"

// GoogleTokenSource returns a token source for a service account key file,
// fetching tokens from tokenURL (the connection's token endpoint) over
// httpClient. ctx must outlive the scan.
func GoogleTokenSource(ctx context.Context, httpClient *http.Client, serviceAccountJSON, tokenURL string) (oauth2.TokenSource, error) {
	cfg, err := google.JWTConfigFromJSON([]byte(serviceAccountJSON), GoogleScope)
	if err != nil {
		return nil, errors.New("the service account key is not a valid JSON key file")
	}
	if tokenURL != "" {
		cfg.TokenURL = tokenURL
	}
	return cfg.TokenSource(context.WithValue(ctx, oauth2.HTTPClient, httpClient)), nil
}

// BearerAuthorizer adds an OAuth access token from ts to each request.
func BearerAuthorizer(ts oauth2.TokenSource) func(ctx context.Context, req *http.Request, body []byte) error {
	return func(_ context.Context, req *http.Request, _ []byte) error {
		tok, err := ts.Token()
		if err != nil {
			return errors.New("the token endpoint refused the service account")
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		return nil
	}
}
