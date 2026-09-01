package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/provider"
)

type fakeAgentClient struct {
	chatFn func(context.Context, provider.ChatRequest) (*provider.ChatResponse, error)
}

func (fakeAgentClient) Name() string { return "fake" }

func (fakeAgentClient) ListModels(context.Context) ([]provider.Model, error) { return nil, nil }

func (fakeAgentClient) Complete(context.Context, provider.CompleteRequest) (*provider.CompleteResponse, error) {
	return nil, nil
}

func (f fakeAgentClient) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	return f.chatFn(ctx, req)
}

func TestRunHeadlessAgentUsesHeadlessDefsAndCommand(t *testing.T) {
	var toolNames []string
	client := fakeAgentClient{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			toolNames = provider.ToolNames(req.Tools)
			return &provider.ChatResponse{
				ToolCalls: []provider.ToolCall{{
					ID:   "1",
					Name: "command",
					Input: map[string]any{
						"content": "git status",
					},
				}},
			}, nil
		},
	}

	output, err := testRunHeadlessAgent(context.Background(), client, false)
	if err != nil {
		t.Fatalf("runHeadlessAgent: %v", err)
	}
	if output.Type != "command" || output.Content != "git status" {
		t.Fatalf("unexpected output: %#v", output)
	}
	if got, want := strings.Join(toolNames, ","), "read,bash,grep,glob,command"; got != want {
		t.Fatalf("expected headless tools %q, got %q", want, got)
	}
}

func TestRunHeadlessAgentFallsBackToBareText(t *testing.T) {
	client := fakeAgentClient{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			if len(req.Tools) != 5 {
				t.Fatalf("expected 5 headless tools, got %d", len(req.Tools))
			}
			return &provider.ChatResponse{Content: "answer"}, nil
		},
	}

	output, err := testRunHeadlessAgent(context.Background(), client, false)
	if err != nil {
		t.Fatalf("runHeadlessAgent: %v", err)
	}
	if output.Type != "text" || output.Content != "answer" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestRunHeadlessAgentApprovalFailureFeedsBackToModel(t *testing.T) {
	tmpDir := t.TempDir()
	cmd := "mkdir -p " + filepath.Join(tmpDir, "nested")
	calls := 0

	client := fakeAgentClient{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			calls++
			switch calls {
			case 1:
				return &provider.ChatResponse{
					ToolCalls: []provider.ToolCall{{
						ID:   "bash-1",
						Name: "bash",
						Input: map[string]any{
							"command": cmd,
						},
					}},
				}, nil
			case 2:
				last := req.Messages[len(req.Messages)-1]
				if len(last.ToolResults) != 1 {
					t.Fatalf("expected one tool result, got %#v", last.ToolResults)
				}
				if !last.ToolResults[0].IsError {
					t.Fatalf("expected failed tool result, got %#v", last.ToolResults[0])
				}
				if !strings.Contains(last.ToolResults[0].Content, "llm_judge is disabled") {
					t.Fatalf("unexpected tool result: %#v", last.ToolResults[0])
				}
				return &provider.ChatResponse{Content: "used a safer path"}, nil
			default:
				t.Fatalf("unexpected chat call %d", calls)
				return nil, nil
			}
		},
	}

	output, err := testRunHeadlessAgent(context.Background(), client, false)
	if err != nil {
		t.Fatalf("runHeadlessAgent: %v", err)
	}
	if output.Type != "text" || output.Content != "used a safer path" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestRunHeadlessAgentJudgeSafeExecutesCommand(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "out.txt")
	cmd := "printf ok > " + outFile
	normalCalls := 0

	client := fakeAgentClient{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			if strings.Contains(req.System, "safety classifier") {
				return &provider.ChatResponse{Content: "safe"}, nil
			}

			normalCalls++
			switch normalCalls {
			case 1:
				return &provider.ChatResponse{
					ToolCalls: []provider.ToolCall{{
						ID:   "bash-1",
						Name: "bash",
						Input: map[string]any{
							"command": cmd,
						},
					}},
				}, nil
			case 2:
				last := req.Messages[len(req.Messages)-1]
				if len(last.ToolResults) != 1 || last.ToolResults[0].IsError {
					t.Fatalf("expected successful bash result, got %#v", last.ToolResults)
				}
				var result struct {
					ExitCode int `json:"exit_code"`
				}
				if err := json.Unmarshal([]byte(last.ToolResults[0].Content), &result); err != nil {
					t.Fatalf("unmarshal bash result: %v", err)
				}
				if result.ExitCode != 0 {
					t.Fatalf("expected exit code 0, got %d", result.ExitCode)
				}
				return &provider.ChatResponse{Content: "done"}, nil
			default:
				t.Fatalf("unexpected normal chat call %d", normalCalls)
				return nil, nil
			}
		},
	}

	output, err := testRunHeadlessAgent(context.Background(), client, true)
	if err != nil {
		t.Fatalf("runHeadlessAgent: %v", err)
	}
	if output.Content != "done" {
		t.Fatalf("unexpected output: %#v", output)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("expected file contents %q, got %q", "ok", string(data))
	}
}

func TestRunHeadlessAgentJudgeUnsafeReturnsToolError(t *testing.T) {
	cmd := "printf ok > out.txt"
	normalCalls := 0

	client := fakeAgentClient{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			if strings.Contains(req.System, "safety classifier") {
				return &provider.ChatResponse{Content: "unsafe"}, nil
			}

			normalCalls++
			switch normalCalls {
			case 1:
				return &provider.ChatResponse{
					ToolCalls: []provider.ToolCall{{
						ID:   "bash-1",
						Name: "bash",
						Input: map[string]any{
							"command": cmd,
						},
					}},
				}, nil
			case 2:
				last := req.Messages[len(req.Messages)-1]
				if len(last.ToolResults) != 1 || !last.ToolResults[0].IsError {
					t.Fatalf("expected failed tool result, got %#v", last.ToolResults)
				}
				if !strings.Contains(last.ToolResults[0].Content, "not auto-approved") {
					t.Fatalf("unexpected tool result: %#v", last.ToolResults[0])
				}
				return &provider.ChatResponse{Content: "blocked"}, nil
			default:
				t.Fatalf("unexpected normal chat call %d", normalCalls)
				return nil, nil
			}
		},
	}

	output, err := testRunHeadlessAgent(context.Background(), client, true)
	if err != nil {
		t.Fatalf("runHeadlessAgent: %v", err)
	}
	if output.Content != "blocked" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func testRunHeadlessAgent(ctx context.Context, client provider.AgentClient, llmJudge bool) (agent.FinalResponse, error) {
	return agent.RunHeadless(ctx, newAskHeadlessConfig(runtimeContext{
		client:   client,
		modelID:  "model",
		llmJudge: llmJudge,
	}, "prompt"))
}
