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

package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/resources"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Evaluator source kinds accepted by GenerateEvaluatorSource.
const (
	EvaluatorTypeCode     = "code"
	EvaluatorTypeLLMJudge = "llm_judge"
)

// Evaluation levels accepted by GenerateEvaluatorSource.
const (
	EvaluatorLevelTrace = "trace"
	EvaluatorLevelAgent = "agent"
	EvaluatorLevelLLM   = "llm"
)

// ChatDialect names the request/response shape an upstream speaks. Providers are
// grouped by template handle rather than by URL because the handle is what fixes the
// wire format.
type ChatDialect string

const (
	// DialectOpenAI is POST {base}/chat/completions with an OpenAI-shaped body.
	DialectOpenAI ChatDialect = "openai"
	// DialectAnthropic is POST {base}/v1/messages with an Anthropic-shaped body.
	DialectAnthropic ChatDialect = "anthropic"
)

// dialectsByTemplate maps a provider template handle to the wire format its upstream
// speaks. A handle that is absent cannot be used for generation — better a clear 422
// than a malformed authenticated call that burns the org's quota.
//
// Omitted deliberately:
//   - gemini, awsbedrock: neither accepts either body shape unmodified.
//   - azure-openai, azureai-foundry: Azure does not serve {base}/chat/completions. It
//     requires a deployment-scoped path and an api-version query
//     ({base}/openai/deployments/{deployment}/chat/completions?api-version=…), and its
//     templates carry no endpoint URL, so the deployment name and API version would
//     both have to be guessed. Supporting them needs a real Azure contract in the
//     provider config, not an assumption made here.
var dialectsByTemplate = map[string]ChatDialect{
	"openai":    DialectOpenAI,
	"mistralai": DialectOpenAI,
	"anthropic": DialectAnthropic,
}

// ChatCompletionRequest is one upstream completion call, fully resolved: the caller
// has already decrypted the credential and picked the dialect.
type ChatCompletionRequest struct {
	BaseURL      string
	AuthHeader   string
	AuthValue    string
	Dialect      ChatDialect
	Model        string
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

// LLMChatCompleter performs a single synchronous completion against an LLM upstream
// and returns the assistant's text. It exists as an interface so the generation service
// can be unit tested without a live provider.
type LLMChatCompleter interface {
	Complete(ctx context.Context, req ChatCompletionRequest) (string, error)
}

// GenerateEvaluatorInput carries the console's authoring request. None of it is
// persisted — the whole operation is a pass-through to the provider's upstream.
type GenerateEvaluatorInput struct {
	Model         string
	EvaluatorType string
	Level         string
	Instructions  string
	DisplayName   string
	Description   string
}

// generationMaxTokens caps the completion. An evaluator is a short function or prompt
// template; a larger budget only buys a slower call and a more rambling answer.
const generationMaxTokens = 4096

// EvaluatorCodegenService turns a natural-language instruction into evaluator source by
// calling an org's own LLM provider once. It writes nothing: no record of the provider
// chosen, the model typed, the instruction given, or the source returned.
type EvaluatorCodegenService struct {
	providerService *LLMProviderService
	encryptionKey   []byte
	completer       LLMChatCompleter
	logger          *slog.Logger
}

// NewEvaluatorCodegenService creates a new EvaluatorCodegenService.
func NewEvaluatorCodegenService(
	providerService *LLMProviderService,
	encryptionKey []byte,
	completer LLMChatCompleter,
	logger *slog.Logger,
) *EvaluatorCodegenService {
	return &EvaluatorCodegenService{
		providerService: providerService,
		encryptionKey:   encryptionKey,
		completer:       completer,
		logger:          logger,
	}
}

// GenerateEvaluatorSource resolves the provider, decrypts its upstream credential and
// makes one completion call, returning the generated Python source (for a code
// evaluator) or prompt template (for an llm_judge evaluator).
func (s *EvaluatorCodegenService) GenerateEvaluatorSource(
	ctx context.Context,
	ouID, providerID string,
	in GenerateEvaluatorInput,
) (string, error) {
	if err := validateGenerateInput(in); err != nil {
		return "", err
	}

	// Get resolves the provider within ouID, so a provider belonging to another org
	// surfaces as not-found rather than being read across the tenant boundary.
	provider, err := s.providerService.Get(providerID, ouID)
	if err != nil {
		return "", err
	}

	call, err := s.resolveUpstreamCall(provider)
	if err != nil {
		return "", err
	}
	call.Model = in.Model
	call.SystemPrompt = buildSystemPrompt(in)
	call.UserPrompt = buildUserPrompt(in)
	call.MaxTokens = generationMaxTokens

	// The instruction and the generated source are deliberately absent from this log:
	// they are the user's authoring content, not operational data.
	s.logger.Info("GenerateEvaluatorSource: calling upstream",
		"ouID", ouID, "providerID", providerID, "templateHandle", provider.TemplateHandle,
		"dialect", string(call.Dialect), "evaluatorType", in.EvaluatorType, "level", in.Level)

	raw, err := s.completer.Complete(ctx, call)
	if err != nil {
		return "", err
	}

	source := stripCodeFences(raw)
	if source == "" {
		return "", fmt.Errorf("%w: upstream returned an empty completion", utils.ErrLLMUpstreamFailed)
	}
	return source, nil
}

// resolveUpstreamCall reads the provider's main upstream endpoint and decrypts its
// stored credential into a ready-to-send call.
func (s *EvaluatorCodegenService) resolveUpstreamCall(provider *models.LLMProvider) (ChatCompletionRequest, error) {
	dialect, ok := dialectsByTemplate[provider.TemplateHandle]
	if !ok {
		return ChatCompletionRequest{}, fmt.Errorf(
			"%w: template %q is not supported for generation",
			utils.ErrLLMProviderNotGenerationCapable, provider.TemplateHandle)
	}

	upstream := provider.Configuration.Upstream
	if upstream == nil || upstream.Main == nil || strings.TrimSpace(upstream.Main.URL) == "" {
		return ChatCompletionRequest{}, fmt.Errorf(
			"%w: provider has no upstream endpoint configured", utils.ErrLLMProviderNotGenerationCapable)
	}

	// Validated before the credential is decrypted: this call puts the provider's
	// secret in a request header, so it must not leave the process in cleartext.
	baseURL, err := validateUpstreamURL(upstream.Main.URL)
	if err != nil {
		return ChatCompletionRequest{}, err
	}

	auth := upstream.Main.Auth
	if auth == nil || auth.SecretRef == nil || *auth.SecretRef == "" {
		return ChatCompletionRequest{}, fmt.Errorf(
			"%w: provider has no upstream credential configured", utils.ErrLLMProviderNotGenerationCapable)
	}
	header := utils.StrPointerAsStr(auth.Header, "")
	if header == "" {
		return ChatCompletionRequest{}, fmt.Errorf(
			"%w: provider upstream auth has no header configured", utils.ErrLLMProviderNotGenerationCapable)
	}

	value, err := s.decryptUpstreamSecret(*auth.SecretRef)
	if err != nil {
		return ChatCompletionRequest{}, err
	}

	return ChatCompletionRequest{
		BaseURL:    baseURL,
		AuthHeader: header,
		AuthValue:  value,
		Dialect:    dialect,
	}, nil
}

// validateUpstreamURL parses the provider's stored endpoint and returns it with any
// trailing slash removed.
//
// HTTPS is required because the request carries the provider's decrypted credential
// in a header. Plain HTTP is allowed only for a loopback host, which is how a local
// mock or a developer's own model server is reached and never leaves the machine.
func validateUpstreamURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%w: provider upstream URL is malformed",
			utils.ErrLLMProviderNotGenerationCapable)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(parsed.Hostname()) {
			return "", fmt.Errorf(
				"%w: provider upstream URL must use https, so the provider credential is not sent in cleartext",
				utils.ErrLLMProviderNotGenerationCapable)
		}
	default:
		return "", fmt.Errorf("%w: provider upstream URL must use https, got scheme %q",
			utils.ErrLLMProviderNotGenerationCapable, parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%w: provider upstream URL has no host",
			utils.ErrLLMProviderNotGenerationCapable)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// isLoopbackHost reports whether the host never leaves the local machine.
func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// decryptUpstreamSecret reverses the base64(AES-256-GCM) encoding applied when the
// provider's credential was stored.
func (s *EvaluatorCodegenService) decryptUpstreamSecret(secretRef string) (string, error) {
	encrypted, err := base64.StdEncoding.DecodeString(secretRef)
	if err != nil {
		return "", fmt.Errorf("failed to decode upstream credential: %w", err)
	}
	plaintext, err := utils.DecryptBytes(encrypted, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt upstream credential: %w", err)
	}
	return string(plaintext), nil
}

// Field limits mirroring the maxLength values the OpenAPI contract declares. The
// generated spec types carry no validation of their own, so without these an
// authorized caller could push arbitrarily large strings through prompt
// construction and into the upstream request body.
const (
	maxModelLen        = 200
	maxInstructionsLen = 4000
	maxDisplayNameLen  = 200
	maxDescriptionLen  = 2000
)

// checkLen enforces one field's length cap in runes rather than bytes, so the limit
// matches the character count the spec documents and the user typed.
func checkLen(field, value string, maxLen int) error {
	if len([]rune(value)) > maxLen {
		return fmt.Errorf("%w: %s must be at most %d characters", utils.ErrInvalidInput, field, maxLen)
	}
	return nil
}

func validateGenerateInput(in GenerateEvaluatorInput) error {
	if strings.TrimSpace(in.Model) == "" {
		return fmt.Errorf("%w: model is required", utils.ErrInvalidInput)
	}
	if err := checkLen("model", in.Model, maxModelLen); err != nil {
		return err
	}
	switch in.EvaluatorType {
	case EvaluatorTypeCode, EvaluatorTypeLLMJudge:
	default:
		return fmt.Errorf("%w: evaluatorType must be %q or %q",
			utils.ErrInvalidInput, EvaluatorTypeCode, EvaluatorTypeLLMJudge)
	}
	switch in.Level {
	case EvaluatorLevelTrace, EvaluatorLevelAgent, EvaluatorLevelLLM:
	default:
		return fmt.Errorf("%w: level must be %q, %q or %q",
			utils.ErrInvalidInput, EvaluatorLevelTrace, EvaluatorLevelAgent, EvaluatorLevelLLM)
	}
	if strings.TrimSpace(in.Instructions) == "" {
		return fmt.Errorf("%w: instructions are required", utils.ErrInvalidInput)
	}
	if err := checkLen("instructions", in.Instructions, maxInstructionsLen); err != nil {
		return err
	}
	if err := checkLen("displayName", in.DisplayName, maxDisplayNameLen); err != nil {
		return err
	}
	return checkLen("description", in.Description, maxDescriptionLen)
}

// buildSystemPrompt gives the model the full authoring contract plus the one rule the
// console depends on: the reply is pasted straight into an editor, so it must be the
// artifact and nothing else.
func buildSystemPrompt(in GenerateEvaluatorInput) string {
	var artifact string
	if in.EvaluatorType == EvaluatorTypeCode {
		artifact = "a single Python evaluator function"
	} else {
		artifact = "a single LLM-judge prompt template"
	}

	var b strings.Builder
	b.WriteString("You are an expert at writing WSO2 Agent Management Platform custom evaluators.\n\n")
	b.WriteString("Write ")
	b.WriteString(artifact)
	b.WriteString(" for the ")
	b.WriteString(in.Level)
	b.WriteString(" evaluation level, following the reference below exactly.\n\n")
	b.WriteString("Output rules:\n")
	b.WriteString("- Reply with the ")
	if in.EvaluatorType == EvaluatorTypeCode {
		b.WriteString("Python source")
	} else {
		b.WriteString("prompt template")
	}
	b.WriteString(" only. No prose before or after it, and no Markdown code fences.\n")
	b.WriteString("- Do not invent fields or helpers that the reference does not define.\n")
	if in.EvaluatorType == EvaluatorTypeCode {
		b.WriteString("- Type-hint the first parameter so it fixes the ")
		b.WriteString(in.Level)
		b.WriteString(" level, and return an EvalResult.\n")
	}
	b.WriteString("\n---\n\n")
	b.WriteString(resources.EvaluatorAuthoringGuide)
	return b.String()
}

// buildUserPrompt states what to evaluate, adding the name and description only when
// the form has them — empty headings would read as fields the model should invent.
func buildUserPrompt(in GenerateEvaluatorInput) string {
	var b strings.Builder
	if name := strings.TrimSpace(in.DisplayName); name != "" {
		b.WriteString("Evaluator name: ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	if desc := strings.TrimSpace(in.Description); desc != "" {
		b.WriteString("Evaluator description: ")
		b.WriteString(desc)
		b.WriteString("\n")
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("What it should measure:\n")
	b.WriteString(strings.TrimSpace(in.Instructions))
	return b.String()
}

// stripCodeFences extracts the artifact from a completion that did not obey the
// "no prose, no fences" instruction. Models comply less reliably for prompt
// templates than for code, and a stray preamble line pasted into the editor is a
// defect the user has to notice and clean up by hand.
//
// Two shapes are handled: a fence wrapping the whole reply, and a fence embedded in
// surrounding prose. An unterminated fence is left alone — half a fence is more
// likely a truncated answer than a wrapper, and stripping its opener would silently
// hand back broken source.
func stripCodeFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if block, ok := firstFencedBlock(trimmed); ok {
		return block
	}
	return trimmed
}

// firstFencedBlock returns the contents of the first complete ``` block, if there is
// one. The opening fence may carry a language tag, which is dropped with it.
func firstFencedBlock(s string) (string, bool) {
	lines := strings.Split(s, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			start = i
			break
		}
	}
	if start == -1 {
		return "", false
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "```" {
			return strings.TrimSpace(strings.Join(lines[start+1:i], "\n")), true
		}
	}
	return "", false
}
