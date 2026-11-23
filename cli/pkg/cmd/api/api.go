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

// Package api implements `amctl api`, a low-level passthrough to the Agent
// Manager HTTP API — the escape hatch for endpoints that do not yet have a
// dedicated command. Output and error bodies are streamed verbatim; the
// endpoint path is resolved relative to the instance's /api/v1 base.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

const apiBasePath = "api/v1"

// NewAPICmd builds the `amctl api` command — a low-level authenticated
// passthrough to the Agent Manager HTTP API.
func NewAPICmd(f *cmdutil.Factory) *cobra.Command {
	opts := &APIOptions{
		IO:         f.IOStreams,
		HTTPClient: f.HTTPClient,
		Token:      f.Token,
		BaseURL: func() (string, error) {
			cfg, err := f.Config()
			if err != nil {
				return "", clierr.Newf(clierr.ConfigNotLoaded, "%v", err)
			}
			inst, err := cfg.Current()
			if err != nil {
				return "", clierr.New(clierr.NoInstance, err.Error())
			}
			return inst.URL, nil
		},
	}

	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated request to the Agent Manager API",
		Long: heredoc(`
			Make an authenticated HTTP request to the Agent Manager API and print
			the response verbatim.

			The endpoint is resolved relative to the current instance's API base, so
			"/api/v1" may be omitted: "amctl api /orgs" and "amctl api /api/v1/orgs"
			are equivalent. A module's API is reached at its own root, which is kept as
			given: "amctl api /m/ai-gateway/llm-providers". The current
			instance's access token is attached automatically.

			Resources are scoped under /orgs/{org}/projects/{project}/...; there are no
			flat top-level resources. The {org} and {project} placeholders are filled from
			the current context (--org/--project, then the linked project, then the active
			org), so "amctl api /orgs/{org}/projects/{project}/agents" works from a linked
			directory without spelling out names. An unknown placeholder, or one that
			cannot be resolved, is reported as an error.

			The HTTP method defaults to GET, switching to POST when fields (-f/-F) or a
			request body (--input) are supplied; override it with -X/--method.

			Fields given with -f are sent as strings. Fields given with -F are parsed:
			"true"/"false" become booleans, integers become numbers, "null" becomes null,
			and "@path" reads the value from a file ("@-" reads stdin). For GET, fields are
			added to the query string; otherwise they form a JSON request body.

			The response body is streamed to stdout exactly as received. On an HTTP status
			of 400 or above the body is still printed, a short "HTTP <status>" line is
			written to stderr, and the command exits non-zero.`),
		Example: heredoc(`
			# List organizations
			$ amctl api /orgs

			# List agents, filling {org}/{project} from the current context
			$ amctl api /orgs/{org}/projects/{project}/agents

			# Get one agent (org/project spelled out explicitly)
			$ amctl api /orgs/default/projects/default/agents/it-helpdesk-agent

			# List LLM providers, from the AI Gateway module's API
			$ amctl api /m/ai-gateway/llm-providers

			# Filter with query parameters (explicit GET keeps fields in the query)
			$ amctl api -X GET /orgs/{org}/projects/{project}/agents -f limit=10

			# Create a project from typed fields (inferred POST + JSON body)
			$ amctl api /orgs/{org}/projects -f name=triage -F retain=true

			# Send a raw JSON body from a file or stdin
			$ amctl api -X POST /orgs/{org}/projects --input ./project.json
			$ cat project.json | amctl api -X POST /orgs/{org}/projects --input -

			# Inspect response headers
			$ amctl api -i /orgs/{org}/projects/{project}/agents`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RequestPath = args[0]
			opts.RequestMethodPassed = cmd.Flags().Changed("method")
			opts.Scope = func() (string, string, error) {
				return f.ResolveOrgProject(cmd, false, false)
			}
			return runAPI(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.RequestMethod, "method", "X", "GET", "The HTTP method for the request")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in `key=value` format")
	cmd.Flags().StringArrayVarP(&opts.MagicFields, "field", "F", nil, "Add a typed parameter in `key=value` format (bool/int/null, or @file/@- to read a value)")
	cmd.Flags().StringArrayVarP(&opts.RequestHeaders, "header", "H", nil, "Add a request header in `key:value` format")
	cmd.Flags().StringVar(&opts.RequestInputFile, "input", "", "`file` to send as the request body (use \"-\" for stdin)")
	cmd.Flags().BoolVarP(&opts.ShowResponseHeaders, "include", "i", false, "Include the response status line and headers in the output")
	cmd.Flags().String("project", "", "Override the project used to fill the {project} placeholder")

	return cmd
}

// heredoc trims a leading newline and the leading tab indentation shared by the
// block, so command help can be written as an indented raw string literal.
func heredoc(s string) string {
	s = strings.TrimPrefix(s, "\n")
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimPrefix(ln, "\t\t\t")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// APIOptions carries everything runAPI needs: its dependencies (resolved off
// the Factory) and the parsed request flags.
type APIOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() *http.Client
	Token      func(context.Context) (string, error)
	BaseURL    func() (string, error)
	// Scope resolves the (org, project) used to fill {org}/{project} path
	// placeholders. It is only invoked when the endpoint contains placeholders.
	Scope func() (org, project string, err error)

	RequestPath         string
	RequestMethod       string
	RequestMethodPassed bool
	RawFields           []string
	MagicFields         []string
	RequestHeaders      []string
	RequestInputFile    string
	ShowResponseHeaders bool
}

// runAPI builds and sends the request, then streams the response verbatim.
// Output and error bodies are passed through unchanged; on an HTTP status of
// 400+ it prints a short status line to stderr and returns a rendered error so
// the process exits non-zero without re-printing.
func runAPI(ctx context.Context, o *APIOptions) error {
	bail := func(code, format string, args ...any) error {
		return render.Error(o.IO, render.Scope{}, clierr.Newf(code, format, args...))
	}

	if o.RequestInputFile == "-" && fieldReadsStdin(o.MagicFields) {
		return bail(clierr.InvalidFlag, "cannot read stdin for both --input - and an @- field")
	}

	params, err := parseFields(o.RawFields, o.MagicFields, o.IO.In)
	if err != nil {
		return bail(clierr.InvalidFlag, "%v", err)
	}

	method := inferMethod(o.RequestMethod, o.RequestMethodPassed, len(params) > 0, o.RequestInputFile)

	endpoint := o.RequestPath
	if hasPlaceholders(endpoint) {
		org, project, serr := o.Scope()
		if serr != nil {
			return bail(clierr.NoOrg, "%v", serr)
		}
		endpoint, err = substituteContext(endpoint, map[string]string{"org": org, "project": project})
		if err != nil {
			return bail(clierr.InvalidFlag, "%v", err)
		}
	}

	baseURL, err := o.BaseURL()
	if err != nil {
		return bail(clierr.NoInstance, "%v", err)
	}
	requestURL, err := resolveURL(baseURL, endpoint)
	if err != nil {
		return bail(clierr.InvalidFlag, "%v", err)
	}

	var body io.Reader
	bodyIsJSON := false
	switch {
	case o.RequestInputFile != "":
		b, err := readInput(o.RequestInputFile, o.IO.In)
		if err != nil {
			return bail(clierr.InvalidFlag, "%v", err)
		}
		requestURL = addQuery(requestURL, params)
		body = bytes.NewReader(b)
	case method == http.MethodGet:
		requestURL = addQuery(requestURL, params)
	case len(params) > 0:
		b, err := json.Marshal(params)
		if err != nil {
			return bail(clierr.Internal, "serialize fields: %v", err)
		}
		body = bytes.NewReader(b)
		bodyIsJSON = true
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return bail(clierr.Transport, "build request: %v", err)
	}

	if err := applyHeaders(req, o.RequestHeaders); err != nil {
		return bail(clierr.InvalidFlag, "%v", err)
	}
	// Only resolve a token when the user gave no Authorization header.
	if req.Header.Get("Authorization") == "" {
		token, err := o.Token(ctx)
		if err != nil {
			return render.Error(o.IO, render.Scope{}, err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if bodyIsJSON && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}

	resp, err := o.HTTPClient().Do(req)
	if err != nil {
		return bail(clierr.Transport, "%v", err)
	}
	defer resp.Body.Close()

	if o.ShowResponseHeaders {
		printResponseHeaders(o.IO.Out, resp)
	}
	if _, err := io.Copy(o.IO.Out, resp.Body); err != nil {
		return bail(clierr.Transport, "read response: %v", err)
	}

	if resp.StatusCode >= 400 {
		fmt.Fprintf(o.IO.ErrOut, "amctl: HTTP %d\n", resp.StatusCode)
		return render.Rendered(clierr.Newf(clierr.ServerError, "HTTP %d", resp.StatusCode))
	}
	return nil
}

// readInput returns the raw request body referenced by --input. "-" reads
// stdin; anything else is a file path.
func readInput(name string, stdin io.Reader) ([]byte, error) {
	if name == "-" {
		if stdin == nil {
			return nil, fmt.Errorf("no stdin available for --input -")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return b, nil
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read input file %q: %w", name, err)
	}
	return b, nil
}

// applyHeaders parses "Name: value" header flags onto the request.
func applyHeaders(req *http.Request, headers []string) error {
	for _, h := range headers {
		name, value, found := strings.Cut(h, ":")
		if !found {
			return fmt.Errorf("header %q requires a value separated by ':'", h)
		}
		req.Header.Set(name, strings.TrimSpace(value))
	}
	return nil
}

// printResponseHeaders writes the status line and headers (sorted) followed by
// a blank line, mirroring `gh api -i`.
func printResponseHeaders(w io.Writer, resp *http.Response) {
	fmt.Fprintf(w, "%s %s\n", resp.Proto, resp.Status)
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range resp.Header[k] {
			fmt.Fprintf(w, "%s: %s\n", k, v)
		}
	}
	fmt.Fprintln(w)
}

// placeholderRe matches {name} context placeholders in an endpoint path.
var placeholderRe = regexp.MustCompile(`\{(\w+)\}`)

// hasPlaceholders reports whether the endpoint contains any {name} placeholder.
func hasPlaceholders(endpoint string) bool {
	return placeholderRe.MatchString(endpoint)
}

// substituteContext replaces {name} placeholders in the endpoint with values
// from vals (e.g. {org}/{project} resolved from the current context). It errors
// on an unknown placeholder name or one whose resolved value is empty, so a
// typo or missing context fails loudly instead of producing a 404.
func substituteContext(endpoint string, vals map[string]string) (string, error) {
	var bad error
	out := placeholderRe.ReplaceAllStringFunc(endpoint, func(match string) string {
		name := match[1 : len(match)-1]
		v, ok := vals[name]
		if !ok {
			bad = fmt.Errorf("unknown placeholder %s (supported: %s)", match, supportedPlaceholders(vals))
			return match
		}
		if v == "" {
			bad = fmt.Errorf("cannot resolve placeholder %s: no %s in context (set --%s, run `amctl link`, or `amctl login`)", match, name, name)
			return match
		}
		return v
	})
	if bad != nil {
		return "", bad
	}
	return out, nil
}

// supportedPlaceholders renders the known placeholder names for error messages.
func supportedPlaceholders(vals map[string]string) string {
	names := make([]string, 0, len(vals))
	for k := range vals {
		names = append(names, "{"+k+"}")
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// resolveURL joins the instance base URL with a user-supplied endpoint,
// prefixing /api/v1 unless the endpoint already carries it or names a
// module's API (/m/<module>/...).
func resolveURL(baseURL, endpoint string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", fmt.Errorf("no instance URL configured; run `amctl login`")
	}
	path := strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	if path == "" {
		return "", fmt.Errorf("an endpoint path is required")
	}
	if path != apiBasePath && !strings.HasPrefix(path, apiBasePath+"/") && !strings.HasPrefix(path, "m/") {
		path = apiBasePath + "/" + path
	}
	return base + "/" + path, nil
}

// inferMethod returns the HTTP method to use. When the user did not pass an
// explicit method, the presence of fields or a request body implies POST.
func inferMethod(method string, methodPassed, hasParams bool, inputFile string) string {
	if !methodPassed && (hasParams || inputFile != "") {
		return "POST"
	}
	return strings.ToUpper(method)
}

// magicFieldValue converts a -F value into its typed form (bool, int, null,
// @file contents) or returns it unchanged as a string.
func magicFieldValue(v string, stdin io.Reader) (any, error) {
	if strings.HasPrefix(v, "@") {
		return readFileArg(v[1:], stdin)
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	default:
		return v, nil
	}
}

// readFileArg reads the contents referenced by an @-prefixed field value. "-"
// reads from stdin; anything else is a file path.
func readFileArg(name string, stdin io.Reader) (string, error) {
	if name == "-" {
		if stdin == nil {
			return "", fmt.Errorf("no stdin available for @- field")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("read field file %q: %w", name, err)
	}
	return string(b), nil
}

// parseField splits "key=value" into its parts. When magic is true the value
// is type-converted via magicFieldValue.
func parseField(raw string, magic bool, stdin io.Reader) (string, any, error) {
	key, val, found := strings.Cut(raw, "=")
	if !found {
		return "", nil, fmt.Errorf("field %q requires a value separated by '='", raw)
	}
	if strings.TrimSpace(key) == "" {
		return "", nil, fmt.Errorf("field %q has an empty key", raw)
	}
	if magic {
		v, err := magicFieldValue(val, stdin)
		if err != nil {
			return "", nil, err
		}
		return key, v, nil
	}
	return key, val, nil
}

// fieldReadsStdin reports whether any -F field reads its value from stdin (a
// value of exactly "@-"). Raw -f fields are literal, so they never read stdin.
func fieldReadsStdin(magicFields []string) bool {
	for _, f := range magicFields {
		if _, val, found := strings.Cut(f, "="); found && val == "@-" {
			return true
		}
	}
	return false
}

// parseFields collects raw (-f) and magic (-F) fields into a single map.
func parseFields(rawFields, magicFields []string, stdin io.Reader) (map[string]any, error) {
	params := make(map[string]any, len(rawFields)+len(magicFields))
	for _, f := range rawFields {
		k, v, err := parseField(f, false, stdin)
		if err != nil {
			return nil, err
		}
		params[k] = v
	}
	for _, f := range magicFields {
		k, v, err := parseField(f, true, stdin)
		if err != nil {
			return nil, err
		}
		params[k] = v
	}
	return params, nil
}

// addQuery appends params to path as a URL query string, preserving any query
// already present on path.
func addQuery(path string, params map[string]any) string {
	if len(params) == 0 {
		return path
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	q := url.Values{}
	for _, k := range keys {
		q.Set(k, fmt.Sprintf("%v", params[k]))
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + q.Encode()
}
