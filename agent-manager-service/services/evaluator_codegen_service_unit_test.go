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
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// codegenTestKey is a fixed AES-256 key. The value is irrelevant as long as the test
// encrypts with the same key the service decrypts with.
var codegenTestKey = []byte("0123456789abcdef0123456789abcdef")

const (
	codegenTestOU       = "org-uuid"
	codegenTestProvider = "11111111-1111-1111-1111-111111111111"
)

// fakeChatCompleter is a hand-written LLMChatCompleter stub. It records the last call
// so tests can assert what the service resolved before reaching the upstream.
type fakeChatCompleter struct {
	lastReq  ChatCompletionRequest
	called   bool
	response string
	err      error
}

func (f *fakeChatCompleter) Complete(_ context.Context, req ChatCompletionRequest) (string, error) {
	f.called = true
	f.lastReq = req
	return f.response, f.err
}

// encryptForTest mirrors how LLMProviderService stores an upstream credential:
// AES-256-GCM, then base64.
func encryptForTest(t *testing.T, plaintext string) string {
	t.Helper()
	encrypted, err := utils.EncryptBytes([]byte(plaintext), codegenTestKey)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(encrypted)
}

// validProvider returns an OpenAI-template provider with a usable upstream.
func validProvider(t *testing.T) *models.LLMProvider {
	t.Helper()
	secretRef := encryptForTest(t, "Bearer sk-test-credential")
	return &models.LLMProvider{
		UUID:           uuid.MustParse(codegenTestProvider),
		TemplateHandle: "openai",
		Configuration: models.LLMProviderConfig{
			Name:   "OpenAI",
			Handle: "openai-main",
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					// Trailing slash is deliberate: the service must not emit "//".
					URL: "https://api.openai.com/v1/",
					Auth: &models.UpstreamAuth{
						Header:    strPtr("Authorization"),
						SecretRef: &secretRef,
					},
				},
			},
		},
	}
}

// newCodegenService builds the service over a provider repo that always returns the
// given provider, plus the supplied completer.
func newCodegenService(provider *models.LLMProvider, completer LLMChatCompleter) *EvaluatorCodegenService {
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(_ string, _ string) (*models.LLMProvider, error) {
			if provider == nil {
				return nil, gorm.ErrRecordNotFound
			}
			return provider, nil
		},
	}
	providerService := NewLLMProviderService(
		nil, providerRepo, nil, nil, nil, nil, codegenTestKey, nil, nil, nil, nil, nil,
	)
	return NewEvaluatorCodegenService(providerService, codegenTestKey, completer, discardLogger())
}

// validInput is a request that passes every validation gate.
func validInput() GenerateEvaluatorInput {
	return GenerateEvaluatorInput{
		Model:         "gpt-4o",
		EvaluatorType: EvaluatorTypeCode,
		Level:         EvaluatorLevelTrace,
		Instructions:  "Score whether the answer cites a retrieved document.",
	}
}

func TestGenerateEvaluatorSourceRejectsInvalidInput(t *testing.T) {
	cases := map[string]GenerateEvaluatorInput{
		"empty model": func() GenerateEvaluatorInput {
			in := validInput()
			in.Model = "   "
			return in
		}(),
		"unknown evaluator type": func() GenerateEvaluatorInput {
			in := validInput()
			in.EvaluatorType = "regex"
			return in
		}(),
		"unknown level": func() GenerateEvaluatorInput {
			in := validInput()
			in.Level = "span"
			return in
		}(),
		"empty instructions": func() GenerateEvaluatorInput {
			in := validInput()
			in.Instructions = "\n\t "
			return in
		}(),
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			completer := &fakeChatCompleter{}
			svc := newCodegenService(validProvider(t), completer)

			_, err := svc.GenerateEvaluatorSource(context.Background(), codegenTestOU, codegenTestProvider, in)

			assert.ErrorIs(t, err, utils.ErrInvalidInput)
			// Validation must gate the call: a rejected request never spends quota.
			assert.False(t, completer.called, "upstream must not be called for invalid input")
		})
	}
}

func TestGenerateEvaluatorSourceEnforcesFieldLimits(t *testing.T) {
	cases := map[string]GenerateEvaluatorInput{
		"model too long": func() GenerateEvaluatorInput {
			in := validInput()
			in.Model = strings.Repeat("m", maxModelLen+1)
			return in
		}(),
		"instructions too long": func() GenerateEvaluatorInput {
			in := validInput()
			in.Instructions = strings.Repeat("i", maxInstructionsLen+1)
			return in
		}(),
		"displayName too long": func() GenerateEvaluatorInput {
			in := validInput()
			in.DisplayName = strings.Repeat("d", maxDisplayNameLen+1)
			return in
		}(),
		"description too long": func() GenerateEvaluatorInput {
			in := validInput()
			in.Description = strings.Repeat("d", maxDescriptionLen+1)
			return in
		}(),
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			completer := &fakeChatCompleter{}
			svc := newCodegenService(validProvider(t), completer)

			_, err := svc.GenerateEvaluatorSource(context.Background(), codegenTestOU, codegenTestProvider, in)

			assert.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.False(t, completer.called, "oversized input must not reach the upstream")
		})
	}
}

func TestGenerateEvaluatorSourceCountsLimitsInRunes(t *testing.T) {
	// A multi-byte string at exactly the rune cap must pass: counting bytes would
	// reject legitimate non-ASCII instructions well before the documented limit.
	completer := &fakeChatCompleter{response: "def my_evaluator(): ..."}
	svc := newCodegenService(validProvider(t), completer)

	in := validInput()
	in.Instructions = strings.Repeat("é", maxInstructionsLen)

	_, err := svc.GenerateEvaluatorSource(context.Background(), codegenTestOU, codegenTestProvider, in)

	require.NoError(t, err)
}

func TestGenerateEvaluatorSourceRequiresSecureUpstream(t *testing.T) {
	cases := map[string]struct {
		url       string
		wantError bool
	}{
		"https is allowed":            {url: "https://api.openai.com/v1", wantError: false},
		"plain http is rejected":      {url: "http://api.openai.com/v1", wantError: true},
		"loopback http is allowed":    {url: "http://localhost:11434/v1", wantError: false},
		"loopback ip http is allowed": {url: "http://127.0.0.1:11434/v1", wantError: false},
		"non-http scheme is rejected": {url: "ftp://api.openai.com/v1", wantError: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			provider := validProvider(t)
			provider.Configuration.Upstream.Main.URL = tc.url
			completer := &fakeChatCompleter{response: "def my_evaluator(): ..."}
			svc := newCodegenService(provider, completer)

			_, err := svc.GenerateEvaluatorSource(
				context.Background(), codegenTestOU, codegenTestProvider, validInput())

			if tc.wantError {
				assert.ErrorIs(t, err, utils.ErrLLMProviderNotGenerationCapable)
				// The credential must not be sent over a cleartext channel.
				assert.False(t, completer.called, "insecure upstream must not receive the credential")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGenerateEvaluatorSourceRejectsAzureTemplates(t *testing.T) {
	// Azure needs a deployment-scoped path and an api-version query that this
	// endpoint has no contract for, so it must refuse rather than guess.
	for _, handle := range []string{"azure-openai", "azureai-foundry"} {
		t.Run(handle, func(t *testing.T) {
			provider := validProvider(t)
			provider.TemplateHandle = handle
			completer := &fakeChatCompleter{}
			svc := newCodegenService(provider, completer)

			_, err := svc.GenerateEvaluatorSource(
				context.Background(), codegenTestOU, codegenTestProvider, validInput())

			assert.ErrorIs(t, err, utils.ErrLLMProviderNotGenerationCapable)
			assert.False(t, completer.called)
		})
	}
}

func TestGenerateEvaluatorSourceRejectsUnusableProvider(t *testing.T) {
	secretRef := encryptForTest(t, "Bearer sk-test-credential")

	cases := map[string]func(p *models.LLMProvider){
		"unsupported template": func(p *models.LLMProvider) {
			// Gemini speaks neither the OpenAI nor the Anthropic body shape.
			p.TemplateHandle = "gemini"
		},
		"no upstream block": func(p *models.LLMProvider) {
			p.Configuration.Upstream = nil
		},
		"no upstream url": func(p *models.LLMProvider) {
			p.Configuration.Upstream.Main.URL = "  "
		},
		"no credential": func(p *models.LLMProvider) {
			p.Configuration.Upstream.Main.Auth = nil
		},
		"empty credential": func(p *models.LLMProvider) {
			empty := ""
			p.Configuration.Upstream.Main.Auth.SecretRef = &empty
		},
		"no auth header": func(p *models.LLMProvider) {
			p.Configuration.Upstream.Main.Auth = &models.UpstreamAuth{SecretRef: &secretRef}
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			provider := validProvider(t)
			mutate(provider)
			completer := &fakeChatCompleter{}
			svc := newCodegenService(provider, completer)

			_, err := svc.GenerateEvaluatorSource(
				context.Background(), codegenTestOU, codegenTestProvider, validInput())

			assert.ErrorIs(t, err, utils.ErrLLMProviderNotGenerationCapable)
			assert.False(t, completer.called, "upstream must not be called for an unusable provider")
		})
	}
}

func TestGenerateEvaluatorSourceResolvesUpstreamCall(t *testing.T) {
	completer := &fakeChatCompleter{response: "def my_evaluator(trace: Trace) -> EvalResult: ..."}
	svc := newCodegenService(validProvider(t), completer)

	in := validInput()
	in.Model = "gpt-4o-mini"
	in.DisplayName = "Citation check"
	in.Description = "Checks grounding"

	source, err := svc.GenerateEvaluatorSource(context.Background(), codegenTestOU, codegenTestProvider, in)

	require.NoError(t, err)
	assert.Equal(t, "def my_evaluator(trace: Trace) -> EvalResult: ...", source)

	req := completer.lastReq
	assert.Equal(t, DialectOpenAI, req.Dialect)
	// The trailing slash on the configured URL must not survive into the call.
	assert.Equal(t, "https://api.openai.com/v1", req.BaseURL)
	assert.Equal(t, "Authorization", req.AuthHeader)
	assert.Equal(t, "Bearer sk-test-credential", req.AuthValue, "stored credential must be decrypted")
	// The user-typed model is passed through verbatim, not resolved to a default.
	assert.Equal(t, "gpt-4o-mini", req.Model)
	assert.Contains(t, req.UserPrompt, "Citation check")
	assert.Contains(t, req.UserPrompt, "Checks grounding")
	assert.Contains(t, req.UserPrompt, in.Instructions)
	assert.NotEmpty(t, req.SystemPrompt)
}

func TestGenerateEvaluatorSourceUsesAnthropicDialect(t *testing.T) {
	provider := validProvider(t)
	provider.TemplateHandle = "anthropic"
	provider.Configuration.Upstream.Main.URL = "https://api.anthropic.com"
	completer := &fakeChatCompleter{response: "judge prompt"}
	svc := newCodegenService(provider, completer)

	in := validInput()
	in.EvaluatorType = EvaluatorTypeLLMJudge

	_, err := svc.GenerateEvaluatorSource(context.Background(), codegenTestOU, codegenTestProvider, in)

	require.NoError(t, err)
	assert.Equal(t, DialectAnthropic, completer.lastReq.Dialect)
}

func TestGenerateEvaluatorSourceStripsCodeFences(t *testing.T) {
	cases := map[string]struct {
		completion string
		want       string
	}{
		"fenced with language": {
			completion: "```python\ndef my_evaluator():\n    pass\n```",
			want:       "def my_evaluator():\n    pass",
		},
		"fenced without language": {
			completion: "```\ndef my_evaluator():\n    pass\n```",
			want:       "def my_evaluator():\n    pass",
		},
		"unfenced is untouched": {
			completion: "def my_evaluator():\n    pass",
			want:       "def my_evaluator():\n    pass",
		},
		"fence embedded in prose": {
			// Observed from a real llm_judge generation: the model prefixed a
			// "Variable: ..." line before the fenced template.
			completion: "Variable: `agent_trace` (AgentTrace)\n\n```\nJudge the answer.\n```\n\nUse it as-is.",
			want:       "Judge the answer.",
		},
		"unterminated fence is left alone": {
			// Half a fence is more likely a truncated answer than a wrapper; stripping
			// the opener would silently hand the user broken source.
			completion: "```python\ndef my_evaluator():",
			want:       "```python\ndef my_evaluator():",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			completer := &fakeChatCompleter{response: tc.completion}
			svc := newCodegenService(validProvider(t), completer)

			source, err := svc.GenerateEvaluatorSource(
				context.Background(), codegenTestOU, codegenTestProvider, validInput())

			require.NoError(t, err)
			assert.Equal(t, tc.want, source)
		})
	}
}

func TestGenerateEvaluatorSourceRejectsEmptyCompletion(t *testing.T) {
	completer := &fakeChatCompleter{response: "   \n  "}
	svc := newCodegenService(validProvider(t), completer)

	_, err := svc.GenerateEvaluatorSource(
		context.Background(), codegenTestOU, codegenTestProvider, validInput())

	assert.ErrorIs(t, err, utils.ErrLLMUpstreamFailed)
}

func TestGenerateEvaluatorSourcePropagatesUpstreamError(t *testing.T) {
	upstreamErr := errors.New("connection refused")
	completer := &fakeChatCompleter{err: upstreamErr}
	svc := newCodegenService(validProvider(t), completer)

	_, err := svc.GenerateEvaluatorSource(
		context.Background(), codegenTestOU, codegenTestProvider, validInput())

	assert.ErrorIs(t, err, upstreamErr)
	// A transport failure is not a missing provider — it must not be flattened to 404.
	assert.NotErrorIs(t, err, utils.ErrLLMProviderNotFound)
}

func TestGenerateEvaluatorSourceMapsMissingProvider(t *testing.T) {
	completer := &fakeChatCompleter{}
	svc := newCodegenService(nil, completer)

	_, err := svc.GenerateEvaluatorSource(
		context.Background(), codegenTestOU, codegenTestProvider, validInput())

	assert.ErrorIs(t, err, utils.ErrLLMProviderNotFound)
	assert.False(t, completer.called)
}
