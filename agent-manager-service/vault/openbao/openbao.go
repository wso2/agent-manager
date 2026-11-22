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

// Package openbao stores AMP credentials in a standalone OpenBao KV v2 mount.
package openbao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

const (
	Provider       = "openbao"
	requestTimeout = 3 * time.Second
)

var mountName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func init() {
	config.RegisterKeys("OPENBAO_ADDR", "OPENBAO_TOKEN", "OPENBAO_KV_MOUNT")
	vault.Register(Provider, func(vault.Config) (vault.Vault, error) {
		return New(config.Value("OPENBAO_ADDR"), config.Value("OPENBAO_TOKEN"), config.Value("OPENBAO_KV_MOUNT"))
	})
}

// Vault uses a separate KV path for each write, so a failed SQL transaction
// cannot overwrite the value referenced by a committed row.
type Vault struct {
	endpoint string
	token    string
	client   *http.Client
}

var _ vault.ReferenceVault = (*Vault)(nil)

func New(address, token, mount string) (*Vault, error) {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("OPENBAO_ADDR must be an HTTP or HTTPS origin")
	}
	if strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("OPENBAO_TOKEN must be a valid nonempty token")
	}
	mount = strings.TrimSpace(mount)
	if mount == "" {
		mount = "secret"
	}
	if !mountName.MatchString(mount) {
		return nil, errors.New("OPENBAO_KV_MOUNT must be a single KV v2 mount name")
	}
	return &Vault{
		endpoint: strings.TrimRight(u.String(), "/") + "/v1/" + mount,
		token:    strings.TrimSpace(token),
		client:   &http.Client{Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (*Vault) Provider() string { return Provider }

func scope(ref vault.Ref) (string, error) {
	org, err := uuid.Parse(ref.OrgID)
	if err != nil || org == uuid.Nil || ref.SecretID == uuid.Nil {
		return "", errors.New("OpenBao secret needs an organization and secret ID")
	}
	return "amp/" + org.String() + "/" + ref.SecretID.String() + "/", nil
}

func checkReference(ref vault.Ref, path string) error {
	prefix, err := scope(ref)
	if err != nil {
		return err
	}
	suffix, ok := strings.CutPrefix(path, prefix)
	operation, err := uuid.Parse(suffix)
	if !ok || err != nil || operation == uuid.Nil || operation.String() != suffix {
		return errors.New("OpenBao reference does not match the secret")
	}
	return nil
}

type secretData struct {
	Value []byte `json:"value"`
}

type writeRequest struct {
	Options casOptions `json:"options"`
	Data    secretData `json:"data"`
}

type casOptions struct {
	Version int `json:"cas"`
}

type readResponse struct {
	Data struct {
		Data secretData `json:"data"`
	} `json:"data"`
}

func (*Vault) Reserve(ref vault.Ref) (string, error) {
	prefix, err := scope(ref)
	if err != nil {
		return "", err
	}
	if ref.OperationID == uuid.Nil {
		return "", errors.New("OpenBao write needs an operation ID")
	}
	return prefix + ref.OperationID.String(), nil
}

// Abandon leaves a KV metadata tombstone. CAS=0 on Seal can no longer
// recreate a value even if its original HTTP request reached OpenBao late.
func (v *Vault) Abandon(ctx context.Context, ref vault.Ref, path string) error {
	if err := checkReference(ref, path); err != nil {
		return err
	}
	// If this path was never written, win the CAS with a nonsecret placeholder.
	// If the writer won first, destroy its version instead. Writing to an
	// existing path needs the update capability, which the API's policy does
	// not grant, so OpenBao answers 403 rather than a CAS 400 there. Either
	// way the destroy below decides: it fails too if the token is unusable.
	marker := writeRequest{Options: casOptions{Version: 0}, Data: secretData{Value: nil}}
	err := v.request(ctx, http.MethodPost, "/data/"+path, marker, nil)
	var status requestError
	if err != nil && (!errors.As(err, &status) || (status.code != http.StatusBadRequest && status.code != http.StatusForbidden)) {
		return err
	}
	return v.request(ctx, http.MethodPut, "/destroy/"+path, struct {
		Versions []int `json:"versions"`
	}{Versions: []int{1}}, nil)
}

// healthPath is never written. Reading it exercises the server, the mount and
// the token's read access under the API's policy: a usable vault answers 404.
const healthPath = "/data/amp/healthz"

var _ vault.Checker = (*Vault)(nil)

func (v *Vault) Check(ctx context.Context) error {
	err := v.request(ctx, http.MethodGet, healthPath, nil, nil)
	var status requestError
	if err == nil || (errors.As(err, &status) && status.code == http.StatusNotFound) {
		return nil
	}
	return err
}

func (v *Vault) Seal(ctx context.Context, ref vault.Ref, value []byte, _ *vault.Sealed) (vault.Sealed, error) {
	if len(value) == 0 {
		return vault.Sealed{}, vault.ErrEmptyValue
	}
	path, err := v.Reserve(ref)
	if err != nil {
		return vault.Sealed{}, err
	}
	body := writeRequest{Options: casOptions{Version: 0}, Data: secretData{Value: value}}
	if err := v.request(ctx, http.MethodPost, "/data/"+path, body, nil); err != nil {
		return vault.Sealed{}, err
	}
	return vault.Sealed{Ciphertext: nil, KeyID: "", ExternalRef: path}, nil
}

func (v *Vault) Open(ctx context.Context, ref vault.Ref, sealed vault.Sealed) ([]byte, error) {
	if err := checkReference(ref, sealed.ExternalRef); err != nil {
		return nil, err
	}
	var out readResponse
	if err := v.request(ctx, http.MethodGet, "/data/"+sealed.ExternalRef, nil, &out); err != nil {
		return nil, err
	}
	if len(out.Data.Data.Value) == 0 {
		return nil, errors.New("OpenBao response has no secret value")
	}
	return out.Data.Data.Value, nil
}

func (v *Vault) Destroy(ctx context.Context, ref vault.Ref, sealed vault.Sealed) error {
	if err := checkReference(ref, sealed.ExternalRef); err != nil {
		return err
	}
	return v.request(ctx, http.MethodDelete, "/metadata/"+sealed.ExternalRef, nil, nil)
}

type requestError struct {
	method string
	code   int
}

func (e requestError) Error() string {
	return fmt.Sprintf("OpenBao %s returned HTTP %d", e.method, e.code)
}

func (v *Vault) request(ctx context.Context, method, path string, body, out any) error {
	var source io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode OpenBao request: %w", err)
		}
		source = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, v.endpoint+path, source)
	if err != nil {
		return fmt.Errorf("build OpenBao request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("OpenBao %s: %w", method, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// A provider error body can contain sensitive request data.
		return requestError{method: method, code: res.StatusCode}
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out); err != nil {
			return fmt.Errorf("decode OpenBao response: %w", err)
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	}
	return nil
}
