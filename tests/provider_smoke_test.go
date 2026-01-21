//go:build integration

package tests

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vango-go/vai/pkg/core/types"
	"github.com/vango-go/vai/pkg/core/providers/anthropic"
	"github.com/vango-go/vai/pkg/core/providers/cerebras"
	"github.com/vango-go/vai/pkg/core/providers/gemini"
	"github.com/vango-go/vai/pkg/core/providers/gemini_oauth"
	"github.com/vango-go/vai/pkg/core/providers/groq"
	"github.com/vango-go/vai/pkg/core/providers/oai_resp"
	"github.com/vango-go/vai/pkg/core/providers/openai"
)

func TestProviderSmoke(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	type providerTest struct {
		name string
		run  func(context.Context) error
	}

	var tests []providerTest

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := envOrDefault("VAI_ANTHROPIC_MODEL", "claude-3-5-haiku-20241022")
		p := anthropic.New(key)
		tests = append(tests, providerTest{
			name: "anthropic",
			run: func(ctx context.Context) error {
				_, err := p.CreateMessage(ctx, &types.MessageRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: "Say 'ok' and nothing else."},
					},
					MaxTokens: 16,
				})
				return err
			},
		})
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := envOrDefault("VAI_OPENAI_MODEL", "gpt-4o-mini")
		p := openai.New(key)
		tests = append(tests, providerTest{
			name: "openai",
			run: func(ctx context.Context) error {
				_, err := p.CreateMessage(ctx, &types.MessageRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: "Say 'ok' and nothing else."},
					},
					MaxTokens: 16,
				})
				return err
			},
		})

		respModel := envOrDefault("VAI_OAI_RESP_MODEL", model)
		respProvider := oai_resp.New(key)
		tests = append(tests, providerTest{
			name: "oai-resp",
			run: func(ctx context.Context) error {
				_, err := respProvider.CreateMessage(ctx, &types.MessageRequest{
					Model: respModel,
					Messages: []types.Message{
						{Role: "user", Content: "Say 'ok' and nothing else."},
					},
					MaxTokens: 16,
				})
				return err
			},
		})
	}

	if key := os.Getenv("GROQ_API_KEY"); key != "" {
		model := envOrDefault("VAI_GROQ_MODEL", "llama-3.3-70b-versatile")
		p := groq.New(key)
		tests = append(tests, providerTest{
			name: "groq",
			run: func(ctx context.Context) error {
				_, err := p.CreateMessage(ctx, &types.MessageRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: "Say 'ok' and nothing else."},
					},
					MaxTokens: 16,
				})
				return err
			},
		})
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		geminiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if geminiKey != "" {
		model := envOrDefault("VAI_GEMINI_MODEL", "gemini-3-flash-preview")
		p := gemini.New(geminiKey)
		tests = append(tests, providerTest{
			name: "gemini",
			run: func(ctx context.Context) error {
				_, err := p.CreateMessage(ctx, &types.MessageRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: "Say 'ok' and nothing else."},
					},
					MaxTokens: 16,
				})
				return err
			},
		})
	}

	if projectID := os.Getenv("GEMINI_OAUTH_PROJECT_ID"); projectID != "" {
		provider, err := gemini_oauth.New(gemini_oauth.WithProjectID(projectID))
		if err == nil {
			model := envOrDefault("VAI_GEMINI_OAUTH_MODEL", "gemini-3-flash-preview")
			tests = append(tests, providerTest{
				name: "gemini-oauth",
				run: func(ctx context.Context) error {
					_, err := provider.CreateMessage(ctx, &types.MessageRequest{
						Model: model,
						Messages: []types.Message{
							{Role: "user", Content: "Say 'ok' and nothing else."},
						},
						MaxTokens: 16,
					})
					return err
				},
			})
		}
	}

	if key := os.Getenv("CEREBRAS_API_KEY"); key != "" {
		model := envOrDefault("VAI_CEREBRAS_MODEL", "llama3.1-8b")
		p := cerebras.New(key)
		tests = append(tests, providerTest{
			name: "cerebras",
			run: func(ctx context.Context) error {
				_, err := p.CreateMessage(ctx, &types.MessageRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: "Say 'ok' and nothing else."},
					},
					MaxTokens: 16,
				})
				return err
			},
		})
	}

	if len(tests) == 0 {
		t.Skip("no provider keys found")
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(ctx); err != nil {
				t.Fatalf("provider smoke failed: %v", err)
			}
		})
	}
}

func envOrDefault(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}
