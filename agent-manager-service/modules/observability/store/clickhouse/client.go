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

package clickhouse

import (
	"bufio"
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
)

// client talks to ClickHouse's HTTP interface as one user. Values reach the
// server as query parameters ({name:Type} in the SQL), never spliced in.
type client struct {
	url      string
	database string
	user     string
	password string
	http     *http.Client
}

// validDatabase is a database name safe to put in SQL unquoted.
var validDatabase = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// newClient returns a client of database, or of no database when it is "".
func newClient(rawURL, database, user, password string) (*client, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("clickhouse: url must be http(s)://host:port (MODULE_OBSERVABILITY_CLICKHOUSE_URL)")
	}
	if u.User != nil {
		return nil, fmt.Errorf("clickhouse: url %s must not hold a login; set the user and password settings instead (MODULE_OBSERVABILITY_CLICKHOUSE_URL)", u.Redacted())
	}
	if database != "" && !validDatabase.MatchString(database) {
		return nil, fmt.Errorf("clickhouse: database %q must match %s (MODULE_OBSERVABILITY_CLICKHOUSE_DATABASE)", database, validDatabase)
	}
	return &client{
		url:      strings.TrimRight(rawURL, "/") + "/",
		database: database,
		user:     user,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// do sends sql with its parameters and returns the response body.
func (c *client) do(ctx context.Context, sql string, params map[string]string) (io.ReadCloser, error) {
	q := url.Values{}
	if c.database != "" {
		q.Set("database", c.database)
	}
	q.Set("default_format", "JSONEachRow")
	q.Set("output_format_json_quote_64bit_integers", "0")
	for k, v := range params {
		q.Set("param_"+k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"?"+q.Encode(), strings.NewReader(sql))
	if err != nil {
		return nil, err
	}
	if c.user != "" {
		req.Header.Set("X-ClickHouse-User", c.user)
	}
	if c.password != "" {
		req.Header.Set("X-ClickHouse-Key", c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, &Error{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	return resp.Body, nil
}

// exec runs a statement that returns no rows.
func (c *client) exec(ctx context.Context, sql string, params map[string]string) error {
	body, err := c.do(ctx, sql, params)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	_, err = io.Copy(io.Discard, body)
	return err
}

// query runs sql and hands each JSONEachRow line to row. A query that fails
// after its first rows ends with an {"exception": ...} line instead, under
// status 200.
func (c *client) query(ctx context.Context, sql string, params map[string]string, row func([]byte) error) error {
	body, err := c.do(ctx, sql, params)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if msg, ok := streamException(line); ok {
			return &Error{Status: http.StatusOK, Message: msg}
		}
		if err := row(line); err != nil {
			return fmt.Errorf("clickhouse: decode row: %w", err)
		}
	}
	return sc.Err()
}

// streamException returns the message of a line that is only an exception.
func streamException(line []byte) (string, bool) {
	if !bytes.Contains(line, []byte(`"exception"`)) {
		return "", false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(line, &obj) != nil || len(obj) != 1 {
		return "", false
	}
	var msg string
	if json.Unmarshal(obj["exception"], &msg) != nil {
		return "", false
	}
	return msg, true
}

// Error is a statement ClickHouse refused.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("clickhouse: status %d: %s", e.Status, e.Message)
}
