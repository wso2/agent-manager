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

package notify

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	shoutrrrsmtp "github.com/nicholas-fedor/shoutrrr/pkg/services/email/smtp"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMTPDialPinsPublicIPAndPreservesTLSHostname(t *testing.T) {
	// The dial address must be the validated IP, never the name that could now
	// resolve somewhere else. A second lookup would fail this resolver assertion.
	client, peer := net.Pipe()
	defer func() { _ = peer.Close() }()
	calls := 0
	d := smtpDialer{
		resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
			calls++
			require.Equal(t, "smtp.example.test", host)
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		dial: func(_ context.Context, network, address string) (net.Conn, error) {
			assert.Equal(t, "tcp", network)
			assert.Equal(t, "8.8.8.8:465", address)
			return client, nil
		},
	}
	conn, err := d.dialContext(context.Background(), "smtp.example.test", 465)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	assert.Equal(t, 1, calls)

	// Observe SNI on a real TLS handshake over the pinned connection.
	certServer := httptest.NewTLSServer(nil)
	cert := certServer.TLS.Certificates[0]
	certServer.Close()
	seen := make(chan string, 1)
	serverDone := make(chan error, 1)
	go func() {
		server := tls.Server(peer, &tls.Config{
			Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12,
			GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
				seen <- hello.ServerName
				return nil, nil //nolint:nilnil // TLS callback uses nil to retain this server config.
			},
		})
		serverDone <- server.Handshake()
	}()
	cfg := &shoutrrrsmtp.Config{Host: "smtp.example.test", SkipTLSVerify: true}
	require.Equal(t, "smtp.example.test", smtpTLSConfig(cfg).ServerName)
	secured := tls.Client(conn, smtpTLSConfig(cfg))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, secured.HandshakeContext(ctx))
	require.NoError(t, <-serverDone)
	assert.Equal(t, "smtp.example.test", <-seen)
}

func TestSMTPDialRejectsAnyPrivateAnswer(t *testing.T) {
	for _, addresses := range [][]netip.Addr{
		{netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")},
	} {
		d := smtpDialer{
			resolve: func(context.Context, string) ([]netip.Addr, error) { return addresses, nil },
			dial: func(context.Context, string, string) (net.Conn, error) {
				t.Error("must not dial any address when an answer is private")
				return nil, errors.New("unexpected dial")
			},
		}
		_, err := d.dialContext(context.Background(), "smtp.example.test", 25)
		require.ErrorContains(t, err, "public addresses")
	}
}

func smtpTestConfig(t *testing.T, address, query string) *shoutrrrsmtp.Config {
	t.Helper()
	u, err := url.Parse("smtp://" + address + "/?fromaddress=from@example.com&toaddresses=a@example.com,b@example.com&" + query)
	require.NoError(t, err)
	service := &shoutrrrsmtp.Service{}
	require.NoError(t, service.Initialize(u, nil))
	return service.Config
}

func TestDeliverSMTPPrivateHostRequiresOptIn(t *testing.T) {
	cfg := smtpTestConfig(t, "127.0.0.1:25", "")
	err := deliverSMTP(context.Background(), cfg, "body", nil, false)
	require.ErrorContains(t, err, "public addresses")
}

func TestDeliverSMTPRecipientsAndSubject(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	messages := make(chan []string, 1)
	failures := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			failures <- err
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = fmt.Fprint(conn, "220 mock SMTP\r\n")
		reader := bufio.NewReader(conn)
		var received []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				failures <- err
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				_, _ = fmt.Fprint(conn, "250 mock\r\n")
			case strings.HasPrefix(line, "MAIL FROM"), strings.HasPrefix(line, "RCPT TO"):
				_, _ = fmt.Fprint(conn, "250 OK\r\n")
			case strings.HasPrefix(line, "DATA"):
				_, _ = fmt.Fprint(conn, "354 send data\r\n")
				var data strings.Builder
				for {
					line, err = reader.ReadString('\n')
					if err != nil {
						failures <- err
						return
					}
					if line == ".\r\n" {
						break
					}
					data.WriteString(line)
				}
				received = append(received, data.String())
				_, _ = fmt.Fprint(conn, "250 accepted\r\n")
			case strings.HasPrefix(line, "QUIT"):
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				messages <- received
				return
			default:
				failures <- fmt.Errorf("unexpected command %s", line)
				return
			}
		}
	}()
	cfg := smtpTestConfig(t, listener.Addr().String(), "usestarttls=no")
	params := types.Params{types.TitleKey: "Quality alert"}
	require.NoError(t, deliverSMTP(context.Background(), cfg, "Score below threshold", &params, true))
	select {
	case err := <-failures:
		require.NoError(t, err)
	case got := <-messages:
		require.Len(t, got, 2)
		assert.Contains(t, got[0], "Subject: Quality alert\r\n")
		assert.Contains(t, got[0], "To: a@example.com\r\n")
		assert.Contains(t, got[1], "To: b@example.com\r\n")
		assert.Contains(t, got[0], "Score below threshold")
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP server did not finish")
	}
}

func TestDeliverSMTPCancellationClosesConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	accepted := make(chan struct{})
	closed := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			closed <- err
			return
		}
		defer func() { _ = conn.Close() }()
		close(accepted)
		_, err = conn.Read(make([]byte, 1)) // Do not send greeting: client must unblock on cancellation.
		closed <- err
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	cfg := smtpTestConfig(t, listener.Addr().String(), "")
	go func() { done <- deliverSMTP(ctx, cfg, "body", nil, true) }()
	<-accepted
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancelled SMTP send remained blocked")
	}
	require.ErrorIs(t, <-closed, io.EOF)
}

func TestSMTPMessageHTMLAndHeaderInjection(t *testing.T) {
	cfg := &shoutrrrsmtp.Config{FromAddress: "from@example.com", Subject: "Alert", UseHTML: true}
	message, err := smtpMessage(cfg, "a@example.com", "x < y")
	require.NoError(t, err)
	assert.Contains(t, string(message), "multipart/alternative")
	assert.Contains(t, string(message), "text/plain")
	assert.Contains(t, string(message), "x &lt; y")
	cfg.Subject = "Alert\r\nBcc: victim@example.com"
	_, err = smtpMessage(cfg, "a@example.com", "body")
	require.ErrorContains(t, err, "newlines")
}
