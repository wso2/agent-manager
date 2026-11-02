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
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/netip"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nicholas-fedor/shoutrrr/pkg/format"
	shoutrrrsmtp "github.com/nicholas-fedor/shoutrrr/pkg/services/email/smtp"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"

	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

// smtpDialer validates all resolved addresses, then dials an IP. The original
// hostname stays in the SMTP configuration for TLS and authentication.
type smtpDialer struct {
	resolve      func(context.Context, string) ([]netip.Addr, error)
	dial         func(context.Context, string, string) (net.Conn, error)
	allowPrivate bool
}

func (d smtpDialer) dialContext(ctx context.Context, host string, port uint16) (net.Conn, error) {
	if d.allowPrivate {
		return d.dial(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	}
	ips, err := d.resolve(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("SMTP host must resolve to public addresses")
	}
	for _, ip := range ips {
		if !ssrf.IsPublicIP(ip) {
			return nil, errors.New("SMTP host must resolve to public addresses")
		}
	}
	for _, ip := range ips {
		conn, err := d.dial(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(int(port))))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("failed to connect to SMTP server")
}

func smtpImplicitTLS(cfg *shoutrrrsmtp.Config) bool {
	return cfg.Encryption == shoutrrrsmtp.EncImplicitTLS ||
		(cfg.Encryption == shoutrrrsmtp.EncAuto && cfg.Port == shoutrrrsmtp.ImplicitTLSPort)
}

func smtpTLSConfig(cfg *shoutrrrsmtp.Config) *tls.Config {
	return &tls.Config{
		ServerName:         cfg.Host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.SkipTLSVerify, // #nosec G402 -- explicit existing destination option.
	}
}

// deliverSMTP owns the connection because Shoutrrr v0.18 cannot accept a
// guarded TCP dialer. It retains the library's parsed settings and auth helpers.
func deliverSMTP(ctx context.Context, config *shoutrrrsmtp.Config, body string, params *types.Params, allowPrivate bool) error {
	cfg := config.Clone()
	resolver := format.NewPropKeyResolver(&cfg)
	if err := resolver.UpdateConfigFromParams(&cfg, params); err != nil {
		return errors.New("invalid SMTP message settings")
	}
	cfg.FixEmailTags()
	timeout := sendTimeout
	if cfg.Timeout > 0 && cfg.Timeout < timeout {
		timeout = cfg.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d := smtpDialer{resolve: ssrf.ResolvePublicIPs, dial: (&net.Dialer{Timeout: timeout}).DialContext, allowPrivate: allowPrivate}
	conn, err := d.dialContext(ctx, cfg.Host, cfg.Port)
	if err != nil {
		return err
	}
	// Capture the raw connection: cancellation must not race reassignment to TLS.
	rawConn := conn
	defer func() { _ = rawConn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return errors.New("failed to set SMTP connection deadline")
		}
	}
	if smtpImplicitTLS(&cfg) {
		secured := tls.Client(conn, smtpTLSConfig(&cfg))
		if err := secured.HandshakeContext(ctx); err != nil {
			return errors.New("SMTP TLS handshake failed")
		}
		conn = secured
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return errors.New("SMTP greeting failed")
	}
	defer func() { _ = client.Close() }()
	host := cfg.ClientHost
	if host == "auto" {
		host, err = os.Hostname()
		if err != nil {
			return errors.New("failed to determine SMTP client hostname")
		}
	}
	if host == "" {
		host = "localhost"
	}
	if err := client.Hello(host); err != nil {
		return errors.New("SMTP handshake failed")
	}
	if cfg.UseStartTLS && !smtpImplicitTLS(&cfg) {
		if supported, _ := client.Extension("STARTTLS"); supported {
			if err := client.StartTLS(smtpTLSConfig(&cfg)); err != nil {
				return errors.New("SMTP STARTTLS failed")
			}
		} else if cfg.RequireStartTLS {
			return errors.New("SMTP server does not support required STARTTLS")
		}
	}
	if err := smtpAuthenticate(client, &cfg); err != nil {
		return err
	}
	for _, recipient := range cfg.ToAddresses {
		message, err := smtpMessage(&cfg, recipient, body)
		if err != nil {
			return err
		}
		if err := client.Mail(cfg.FromAddress); err != nil {
			return errors.New("SMTP sender rejected")
		}
		if err := client.Rcpt(recipient); err != nil {
			return errors.New("SMTP recipient rejected")
		}
		w, err := client.Data()
		if err != nil {
			return errors.New("SMTP DATA rejected")
		}
		_, writeErr := w.Write(message)
		closeErr := w.Close()
		if writeErr != nil || closeErr != nil {
			return errors.New("SMTP message delivery failed")
		}
	}
	if err := client.Quit(); err != nil && !strings.Contains(err.Error(), "short response") {
		return errors.New("SMTP session close failed")
	}
	return nil
}

func smtpAuthenticate(client *smtp.Client, cfg *shoutrrrsmtp.Config) error {
	var auth smtp.Auth
	switch cfg.Auth {
	case shoutrrrsmtp.AuthNone:
		return nil
	case shoutrrrsmtp.AuthPlain:
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	case shoutrrrsmtp.AuthCRAMMD5:
		auth = smtp.CRAMMD5Auth(cfg.Username, cfg.Password)
	case shoutrrrsmtp.AuthOAuth2:
		auth = shoutrrrsmtp.OAuth2Auth(cfg.Username, cfg.Password)
	case shoutrrrsmtp.AuthLogin:
		auth = shoutrrrsmtp.LoginAuth(cfg.Username, cfg.Password, cfg.Host)
	default:
		return errors.New("unsupported SMTP authentication type")
	}
	// SMTP replies can echo credentials; never propagate server text.
	if err := client.Auth(auth); err != nil {
		return errors.New("SMTP authentication failed")
	}
	return nil
}

func smtpMessage(cfg *shoutrrrsmtp.Config, recipient, body string) ([]byte, error) {
	if strings.ContainsAny(cfg.Subject+cfg.FromName+cfg.FromAddress+recipient, "\r\n") {
		return nil, errors.New("SMTP headers must not contain newlines")
	}
	from := (&mail.Address{Name: cfg.FromName, Address: cfg.FromAddress}).String()
	var out bytes.Buffer
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\n",
		from, recipient, mime.QEncoding.Encode("UTF-8", cfg.Subject), time.Now().Format(time.RFC1123Z))
	if !cfg.UseHTML {
		out.WriteString("Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		if err := writeSMTPBody(&out, body); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	parts := multipart.NewWriter(&out)
	fmt.Fprintf(&out, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", parts.Boundary())
	for _, part := range []struct{ contentType, text string }{
		{"text/plain; charset=UTF-8", body},
		{"text/html; charset=UTF-8", "<pre>" + html.EscapeString(body) + "</pre>"},
	} {
		w, err := parts.CreatePart(textproto.MIMEHeader{"Content-Type": {part.contentType}, "Content-Transfer-Encoding": {"quoted-printable"}})
		if err != nil {
			return nil, errors.New("failed to encode SMTP message")
		}
		if err := writeSMTPBody(w, part.text); err != nil {
			return nil, err
		}
	}
	if err := parts.Close(); err != nil {
		return nil, errors.New("failed to encode SMTP message")
	}
	return out.Bytes(), nil
}

func writeSMTPBody(w io.Writer, body string) error {
	encoded := quotedprintable.NewWriter(w)
	if _, err := io.WriteString(encoded, body); err != nil {
		return errors.New("failed to encode SMTP body")
	}
	if err := encoded.Close(); err != nil {
		return errors.New("failed to encode SMTP body")
	}
	return nil
}
