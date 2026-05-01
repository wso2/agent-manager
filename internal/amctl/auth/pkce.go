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

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/wso2/agent-manager/internal/amctl/iostreams"
)

const (
	callbackPath        = "/callback"
	defaultCallbackPort = 10325
)

const successHTML = `<!DOCTYPE html>
<html><head><title>Login Successful</title></head>
<body><h1>Login successful</h1><p>You may close this tab.</p></body></html>`

const errorHTML = `<!DOCTYPE html>
<html><head><title>Login Failed</title></head>
<body><h1>Login failed</h1><p>%s</p></body></html>`

type callbackResult struct {
	code string
	err  error
}

func authCodePKCE(ctx context.Context, cfg *oauth2.Config, io *iostreams.IOStreams, openBrowser func(string) error) (*oauth2.Token, error) {
	listenAddr := fmt.Sprintf("127.0.0.1:%d", defaultCallbackPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("start callback listener on %s: %w", listenAddr, err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d%s", port, callbackPath)

	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}

	authURL := cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

	fmt.Fprintf(io.ErrOut, "Redirect URI: %s\n", cfg.RedirectURL)

	if io.CanPrompt() {
		fmt.Fprintf(io.ErrOut, "\nPress Enter to open the browser...")
		buf := make([]byte, 1)
		if _, err := io.In.Read(buf); err != nil {
			return nil, fmt.Errorf("aborted: %w", err)
		}
	}

	if berr := openBrowser(authURL); berr != nil {
		fmt.Fprintf(io.ErrOut, "\nCould not open browser: %v\n\nOpen this URL in your browser:\n\n  %s\n\n", berr, authURL)
	} else {
		fmt.Fprintf(io.ErrOut, "\nBrowser opened. Waiting for authentication...\n")
	}

	resultCh := make(chan callbackResult, 1)
	send := func(r callbackResult) {
		select {
		case resultCh <- r:
		default:
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if got := r.FormValue("state"); got != state {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, errorHTML, "State mismatch — possible CSRF attack.")
			send(callbackResult{err: fmt.Errorf("state mismatch")})
			return
		}
		if errCode := r.FormValue("error"); errCode != "" {
			desc := r.FormValue("error_description")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, errorHTML, desc)
			send(callbackResult{err: fmt.Errorf("authorization error: %s: %s", errCode, desc)})
			return
		}
		code := r.FormValue("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, errorHTML, "No authorization code received.")
			send(callbackResult{err: fmt.Errorf("no authorization code in callback")})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, successHTML)
		send(callbackResult{code: code})
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			send(callbackResult{err: fmt.Errorf("callback server: %w", err)})
		}
	}()

	var result callbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		result = callbackResult{err: ctx.Err()}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(io.ErrOut, "warning: callback server shutdown: %v\n", err)
	}

	if result.err != nil {
		return nil, result.err
	}

	tok, err := cfg.Exchange(ctx, result.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	return tok, nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
