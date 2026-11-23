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

package llmprovider

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

const (
	// defaultVersion must satisfy the spec's `^v\d+\.\d+$` pattern. "v1" does not,
	// and was accepted only because the server did not validate it.
	defaultVersion = "v1.0"
	defaultContext = "/"
	// defaultAuthType matches the auth scheme of the built-in templates that
	// require a credential (openai, anthropic, mistralai, …). It is only sent
	// when the user also supplies a key or an explicit auth override.
	defaultAuthType = "api-key"
	// defaultAccessMode matches what the console sends. A deployed proxy defaults
	// to deny_all server-side and is then unreachable, so a provider created
	// without an explicit mode would be created correctly and still not work.
	defaultAccessMode = "allow_all"
)

// validAuthTypes are the upstream auth schemes accepted by the service.
var validAuthTypes = []string{"api-key", "none"}

// validAccessModes are the access control modes accepted by the service.
var validAccessModes = []string{"allow_all", "deny_all"}

// versionRegex mirrors the spec's `version` pattern.
var versionRegex = regexp.MustCompile(`^v\d+\.\d+$`)

type CreateOptions struct {
	IO *iostreams.IOStreams
	// Client resolves --gateways names; gateways are still a v1 resource.
	Client         func(context.Context) (*aigateway.ClientWithResponses, error)
	ProviderClient func(context.Context) (*aigateway.ClientWithResponses, error)
	ResolveScope   func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope      func(org, proj string) render.Scope

	Org   string
	Scope render.Scope

	ID          string
	DisplayName string
	Version     string
	Context     string
	Template    string
	Description string

	// Upstream overrides. When omitted, the provider inherits the template's
	// endpoint URL and auth scheme.
	UpstreamURL   string
	AuthType      string
	AuthHeader    string
	AuthTypeSet   bool
	AuthHeaderSet bool

	APIKey      string
	APIKeyStdin bool

	AccessMode string
	Gateways   []string
}

// keyRequested reports whether the user asked to attach a credential, without
// reading stdin (so it is safe to call during validation).
func (o *CreateOptions) keyRequested() bool {
	return o.APIKey != "" || o.APIKeyStdin
}

// accessMode falls back to the default so a caller that builds CreateOptions
// directly cannot send an empty mode, which the service would reject.
func (o *CreateOptions) accessMode() string {
	if o.AccessMode == "" {
		return defaultAccessMode
	}
	return o.AccessMode
}

func validateCreate(opts *CreateOptions) error {
	var v []string

	if opts.ID == "" {
		v = append(v, "id argument is required")
	} else if msg := handleViolation(opts.ID); msg != "" {
		v = append(v, msg)
	}
	if opts.DisplayName == "" {
		v = append(v, "--display-name is required")
	}
	if opts.Template == "" {
		v = append(v, "--template is required")
	}
	if msg := contextViolation(opts.Context); msg != "" {
		v = append(v, msg)
	}
	if !versionRegex.MatchString(opts.Version) {
		v = append(v, fmt.Sprintf("--version must match %s, e.g. %s", versionRegex, defaultVersion))
	}
	if !isValidAuthType(opts.AuthType) {
		v = append(v, fmt.Sprintf("--auth-type must be one of: %s", strings.Join(validAuthTypes, ", ")))
	}
	if !slices.Contains(validAccessModes, opts.accessMode()) {
		v = append(v, fmt.Sprintf("--access-mode must be one of: %s", strings.Join(validAccessModes, ", ")))
	}
	if opts.APIKey != "" && opts.APIKeyStdin {
		v = append(v, "--api-key and --api-key-stdin are mutually exclusive")
	}
	if opts.APIKey != "" && strings.TrimSpace(opts.APIKey) == "" {
		v = append(v, "--api-key must not be blank")
	}
	if opts.keyRequested() && opts.AuthType == "none" {
		v = append(v, "an API key cannot be used with --auth-type none")
	}
	for _, g := range opts.Gateways {
		// Identical messages collapse: three blank values are one mistake.
		if msg := gatewayViolation(g); msg != "" && !slices.Contains(v, msg) {
			v = append(v, msg)
		}
	}

	if len(v) == 0 {
		return nil
	}
	return cmdutil.FlagErrors(v)
}

func isValidAuthType(t string) bool {
	return slices.Contains(validAuthTypes, t)
}

// gatewayNameMaxLen mirrors the spec's maxLength on GatewayResponse.name.
const gatewayNameMaxLen = 64

// gatewayViolation returns a validation message when value can be neither a gateway
// UUID nor a gateway name, or "" when it could be either. Rejecting the impossible
// shapes here keeps them from costing a paginated walk of every gateway in the org
// before resolution fails.
//
// It stops short of the spec's `^[a-z0-9-]+$` name pattern on purpose: a plausible
// name still has to be resolved against the server, so mirroring the pattern buys
// nothing beyond these checks and makes the CLI reject valid input the day the server
// relaxes the rule. The checks kept are the ones cmdutil.ValidatePathParam already
// applies to the gateway passed to `amctl gateway get`, plus the length bound.
func gatewayViolation(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "--gateways must not contain a blank value"
	}
	if _, err := uuid.Parse(trimmed); err == nil {
		return ""
	}
	if strings.Contains(trimmed, "/") {
		return fmt.Sprintf("--gateways value %q must not contain '/'", trimmed)
	}
	if strings.ContainsFunc(trimmed, unicode.IsSpace) {
		return fmt.Sprintf("--gateways value %q must not contain whitespace", trimmed)
	}
	if len(trimmed) > gatewayNameMaxLen {
		return fmt.Sprintf("--gateways value %q is longer than a gateway name can be (at most %d characters)",
			trimmed, gatewayNameMaxLen)
	}
	return ""
}

// handleRegex matches a valid resource handle: letters, digits, '-' and '_'.
// It mirrors the server-side rule (utils.ValidateTemplateHandle).
var handleRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// handleViolation returns a validation message when id is not a valid resource
// handle, or "" when it is valid: at most 255 characters and limited to
// letters, digits, '-' and '_'. The empty case is reported separately so the
// caller can emit the "id argument is required" message.
func handleViolation(id string) string {
	if len(id) > 255 {
		return "id must be at most 255 characters"
	}
	if !handleRegex.MatchString(id) {
		return "id must contain only letters, digits, '-' or '_'"
	}
	return ""
}

// contextViolation returns a validation message when the API context path is
// malformed, or "" when valid. The path must start with '/' and must not end
// with '/', except the root path "/". This mirrors the console's validation.
func contextViolation(ctx string) string {
	if !strings.HasPrefix(ctx, "/") {
		return "--context must start with '/'"
	}
	if ctx != "/" && strings.HasSuffix(ctx, "/") {
		return "--context must not end with '/'"
	}
	return ""
}

// gatewayNames returns the --gateways values that are not UUIDs, in order and
// deduplicated. Kept separate so a UUID-only --gateways can skip the lookup that
// resolving names would otherwise cost.
func gatewayNames(raw []string) []string {
	names := []string{}
	for _, g := range raw {
		trimmed := strings.TrimSpace(g)
		if trimmed == "" {
			continue
		}
		if _, err := uuid.Parse(trimmed); err == nil {
			continue
		}
		if !slices.Contains(names, trimmed) {
			names = append(names, trimmed)
		}
	}
	return names
}

// parseGateways converts the raw --gateways values into typed UUIDs, resolving any
// that are names through nameToUUID. A nil nameToUUID rejects every name, which is
// what a UUID-only --gateways gets: resolveGatewayNames skips the lookup entirely.
//
// Duplicates are dropped after resolution, keeping the first occurrence and the
// caller's order: a gateway named twice — or named once by name and once by UUID —
// is one placement, and sending it twice draws a confusing rejection from the
// server's "no two gateways may share an environment" check.
func parseGateways(raw []string, nameToUUID map[string]openapi_types.UUID) ([]openapi_types.UUID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]openapi_types.UUID, 0, len(raw))
	seen := make(map[openapi_types.UUID]struct{}, len(raw))
	for _, g := range raw {
		trimmed := strings.TrimSpace(g)
		if trimmed == "" {
			return nil, fmt.Errorf("--gateways must not contain a blank value")
		}
		id, err := uuid.Parse(trimmed)
		if err != nil {
			resolved, ok := nameToUUID[trimmed]
			if !ok {
				return nil, fmt.Errorf("unknown gateway %q: pass a gateway name or UUID from 'amctl gateway list'", trimmed)
			}
			id = resolved
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func NewCreateCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:             f.IOStreams,
		Client:         f.AIGateway,
		ProviderClient: f.AIGateway,
		ResolveScope:   f.ResolveOrgProject,
		MakeScope:      f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "create <id>",
		Short: "Create a new LLM provider",
		Long: "Create a new LLM provider in an organization.\n\n" +
			"The endpoint URL and auth scheme are inherited from the chosen --template; " +
			"override them with --upstream-url/--auth-type/--auth-header only when needed. " +
			"Supply the provider credential with --api-key-stdin (recommended) or --api-key.\n\n" +
			"Built-in --template handles: anthropic, awsbedrock, azure-openai, azureai-foundry, " +
			"gemini, mistralai, openai. Shell completion on --template lists the live set " +
			"(built-in plus any custom templates) for your org.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.ID = args[0]
			}
			opts.AuthTypeSet = cmd.Flags().Changed("auth-type")
			opts.AuthHeaderSet = cmd.Flags().Changed("auth-header")

			if err := validateCreate(opts); err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			return runCreate(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.DisplayName, "display-name", "", "Human-readable display name (required)")
	cmd.Flags().StringVar(&opts.Template, "template", "", "Provider template handle, e.g. openai, anthropic, mistralai (required)")
	cmd.Flags().StringVar(&opts.Version, "version", defaultVersion, "Provider version")
	cmd.Flags().StringVar(&opts.Context, "context", defaultContext, "API context path (must start with /, no trailing slash)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Provider description")
	cmd.Flags().StringVar(&opts.UpstreamURL, "upstream-url", "", "Override the template's upstream endpoint URL")
	cmd.Flags().StringVar(&opts.AuthType, "auth-type", defaultAuthType, "Upstream auth type: api-key, basic, bearer, or none")
	cmd.Flags().StringVar(&opts.AuthHeader, "auth-header", "", "Override the template's auth header name")
	cmd.Flags().StringVar(&opts.APIKey, "api-key", "", "Provider API key (leaks into shell history; prefer --api-key-stdin)")
	cmd.Flags().BoolVar(&opts.APIKeyStdin, "api-key-stdin", false, "Read the provider API key from stdin")
	cmd.Flags().StringVar(&opts.AccessMode, "access-mode", defaultAccessMode,
		fmt.Sprintf("Route access control for the deployed proxy: %s", strings.Join(validAccessModes, " or ")))
	cmd.Flags().StringSliceVar(&opts.Gateways, "gateways", nil, "Gateway names or UUIDs to deploy the provider to (repeatable)")

	_ = cmd.RegisterFlagCompletionFunc("auth-type", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return validAuthTypes, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("access-mode", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return validAccessModes, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("template", func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return cmdutil.CompleteLLMProviderTemplates(cmd, f), cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("gateways", func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return cmdutil.CompleteGateways(cmd, f), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func runCreate(ctx context.Context, o *CreateOptions) error {
	key := o.APIKey
	if o.APIKeyStdin {
		data, err := io.ReadAll(o.IO.In)
		if err != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.InvalidFlag, "read API key from stdin: %v", err))
		}
		key = strings.TrimSpace(string(data))
		if key == "" {
			return render.Error(o.IO, o.Scope, clierr.New(clierr.InvalidFlag, "no API key provided on stdin"))
		}
	}

	gatewayUUIDs, err := resolveGatewayNames(ctx, o.Client, o.Org, o.Gateways)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	req, err := buildCreateRequest(o, key, gatewayUUIDs)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	providers, err := o.ProviderClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := providers.CreateLLMProviderWithResponse(ctx, req)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON201 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON401, resp.JSON409, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON201)
	}

	printProviderSummary(o.IO, resp.JSON201)
	return nil
}

// resolveGatewayNames maps any non-UUID --gateways values to their UUIDs by listing
// the org's gateways. Returns nil without building a client when every value is
// already a UUID, so the common scripted case costs no extra request.
func resolveGatewayNames(
	ctx context.Context, newClient func(context.Context) (*aigateway.ClientWithResponses, error),
	org string, raw []string,
) (map[string]openapi_types.UUID, error) {
	names := gatewayNames(raw)
	if len(names) == 0 {
		return nil, nil
	}
	client, err := newClient(ctx)
	if err != nil {
		return nil, err
	}

	gateways, err := cmdutil.ListAllGateways(ctx, client, org)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]openapi_types.UUID, len(gateways))
	for _, g := range gateways {
		id, err := uuid.Parse(g.Uuid)
		if err != nil {
			// Skipping the entry would report the gateway as unknown, blaming the
			// user's input for a malformed UUID the server sent.
			return nil, clierr.Newf(clierr.Internal,
				"server returned gateway %q with an unparseable uuid %q", g.Name, g.Uuid)
		}
		byName[g.Name] = id
	}
	return byName, nil
}

// buildCreateRequest maps the resolved options into the provider document. The
// upstream block is attached only when the user supplies a URL or an auth input,
// so a bare `create <id> --display-name --template` defers entirely to the template.
func buildCreateRequest(
	o *CreateOptions, key string, gatewayUUIDs map[string]openapi_types.UUID,
) (aigateway.LlmProviderDocument, error) {
	doc := aigateway.LlmProviderDocument{
		ApiVersion: aigateway.LlmProviderDocumentApiVersion("ai-control-plane.choreolabs.com/v1"),
		Kind:       aigateway.LlmProviderDocumentKindLlmProvider,
		Metadata:   aigateway.DocumentMetadata{Name: o.ID},
	}
	req := &doc.Spec
	req.DisplayName = o.DisplayName
	req.Version = o.Version
	req.Context = o.Context
	req.Template = o.Template
	if o.Description != "" {
		req.Description = &o.Description
	}

	keyProvided := key != ""
	attachAuth := keyProvided || o.AuthTypeSet || o.AuthHeaderSet
	if attachAuth || o.UpstreamURL != "" {
		main := &aigateway.UpstreamEndpoint{}
		if o.UpstreamURL != "" {
			main.Url = &o.UpstreamURL
		}
		if attachAuth {
			auth := &aigateway.UpstreamAuth{Type: aigateway.UpstreamAuthType(o.AuthType)}
			if o.AuthHeader != "" {
				auth.Header = &o.AuthHeader
			}
			if keyProvided {
				auth.Value = &key
			}
			main.Auth = auth
		}
		req.Upstream = aigateway.UpstreamConfig{Main: main}
	}

	gws, err := parseGateways(o.Gateways, gatewayUUIDs)
	if err != nil {
		return doc, cmdutil.FlagErrors([]string{err.Error()})
	}
	if len(gws) > 0 {
		req.Gateways = &gws
	}

	// Sent unconditionally: a deployed proxy defaults to deny_all server-side, so a
	// provider created without an access control block is unreachable.
	req.AccessControl = &aigateway.LLMAccessControl{
		Mode:       aigateway.LLMAccessControlMode(o.accessMode()),
		Exceptions: &[]aigateway.RouteException{},
	}

	return doc, nil
}

func printProviderSummary(ios *iostreams.IOStreams, p *aigateway.LlmProviderDocument) {
	cs := ios.StderrColorScheme()
	fmt.Fprintf(ios.ErrOut, "%s Created LLM provider %s\n\n", cs.SuccessIcon(), p.Metadata.Name)
	fmt.Fprintf(ios.ErrOut, "  Name:     %s\n", p.Spec.DisplayName)
	fmt.Fprintf(ios.ErrOut, "  Template: %s\n", p.Spec.Template)
	fmt.Fprintf(ios.ErrOut, "  Gateways: %s\n", placement(p.Status))
}
