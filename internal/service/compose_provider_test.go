package service

import (
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
)

// TestCompose_ProviderCustom_NoCommand verifies that when provider=custom and
// both entrypoint and command are empty slices, the compose YAML does NOT
// render entrypoint: or command: lines.
func TestCompose_ProviderCustom_NoCommand(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-custom-no-cmd",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Custom No Command",
		Container: "llm-test-custom-nocmd",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{},
		Provider:    "custom",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// entrypoint: line must NOT appear
	if strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should NOT contain 'entrypoint:' when entrypoint is empty:\n%s", yaml)
	}
	// command: line must NOT appear
	if strings.Contains(yaml, "command:") {
		t.Errorf("YAML should NOT contain 'command:' when command is empty:\n%s", yaml)
	}
}

// TestCompose_ProviderCustom_WithCommand verifies that when provider=custom
// and only command is set, the compose YAML renders the command as-is with
// NO vLLM auto-flags appended.
func TestCompose_ProviderCustom_WithCommand(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-custom-with-cmd",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Custom With Command",
		Container: "llm-test-custom-cmd",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{},
		Provider:    "custom",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{"--port", "8000", "--model", "test"},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// command: line must appear
	if !strings.Contains(yaml, "command:") {
		t.Errorf("YAML should contain 'command:' when command is set:\n%s", yaml)
	}
	// command args must be present
	if !strings.Contains(yaml, "--port") || !strings.Contains(yaml, "8000") {
		t.Errorf("YAML should contain command args:\n%s", yaml)
	}
	// vLLM auto-flags must NOT appear (provider is custom)
	vllmFlags := []string{"--max-model-len", "--gpu-memory-utilization", "--max-num-batched-tokens", "--max-num-seqs"}
	for _, flag := range vllmFlags {
		if strings.Contains(yaml, flag) {
			t.Errorf("YAML should NOT contain vLLM auto-flag %q for custom provider:\n%s", flag, yaml)
		}
	}
	// entrypoint: line must NOT appear
	if strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should NOT contain 'entrypoint:' when entrypoint is empty:\n%s", yaml)
	}
}

// TestCompose_ProviderVllm_WithCommand verifies that when provider=vllm and
// command is set, the compose YAML renders the command with vLLM auto-flags
// APPENDED (when model has TotalParamsB and QuantBytesPerParam set).
func TestCompose_ProviderVllm_WithCommand(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	params := 7.0
	quant := 1.0

	model := &models.Model{
		Slug:               "test-vllm-with-cmd",
		Type:               "llm",
		SubType:            "chat",
		Name:               "Test vLLM With Command",
		Container:          "llm-test-vllm-cmd",
		Port:               8080,
		TotalParamsB:       &params,
		QuantBytesPerParam: &quant,
		DefaultContext:     intPtr(4096),
	}

	cfg := EngineComposeConfig{
		Image:       "vllm/vllm-openai:latest",
		Entrypoint:  []string{},
		Provider:    "vllm",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{"--port", "8000"},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// command: line must appear
	if !strings.Contains(yaml, "command:") {
		t.Errorf("YAML should contain 'command:' when command is set:\n%s", yaml)
	}
	// original command args must be present
	if !strings.Contains(yaml, "--port") || !strings.Contains(yaml, "8000") {
		t.Errorf("YAML should contain original command args:\n%s", yaml)
	}
	// vLLM auto-flags should be appended (the model has params set, so mergeProfileFlagsWithOptions
	// will generate flags for a vllm provider)
	// At minimum, --max-model-len or --gpu-memory-utilization should appear
	hasVllmFlag := strings.Contains(yaml, "--max-model-len") ||
		strings.Contains(yaml, "--gpu-memory-utilization") ||
		strings.Contains(yaml, "--max-num-batched-tokens") ||
		strings.Contains(yaml, "--max-num-seqs")
	if !hasVllmFlag {
		t.Errorf("YAML should contain vLLM auto-flags for vllm provider with params set:\n%s", yaml)
	}
}

// TestCompose_ProviderVllm_NoCommand verifies that when provider=vllm and
// both entrypoint and command are empty, the compose YAML does NOT render
// entrypoint: or command: lines.
func TestCompose_ProviderVllm_NoCommand(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-vllm-no-cmd",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test vLLM No Command",
		Container: "llm-test-vllm-nocmd",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "vllm/vllm-openai:latest",
		Entrypoint:  []string{},
		Provider:    "vllm",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// entrypoint: line must NOT appear
	if strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should NOT contain 'entrypoint:' when entrypoint is empty:\n%s", yaml)
	}
	// command: line must NOT appear
	if strings.Contains(yaml, "command:") {
		t.Errorf("YAML should NOT contain 'command:' when command is empty:\n%s", yaml)
	}
}

// TestCompose_ProviderSglang_PassThrough verifies that when provider=sglang,
// both entrypoint and command are rendered as-is with NO auto-flags.
func TestCompose_ProviderSglang_PassThrough(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-sglang-passthrough",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Sglang PassThrough",
		Container: "llm-test-sglang",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "sgl-project/sglang:latest",
		Entrypoint:  []string{"python3", "-m", "sglang.launch_server"},
		Provider:    "sglang",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{"--host", "0.0.0.0", "--port", "8000"},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// entrypoint: line must appear
	if !strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should contain 'entrypoint:' when entrypoint is set:\n%s", yaml)
	}
	// entrypoint items should be single-quoted
	if !strings.Contains(yaml, "'python3'") {
		t.Errorf("YAML should contain single-quoted entrypoint items:\n%s", yaml)
	}
	// command: line must appear
	if !strings.Contains(yaml, "command:") {
		t.Errorf("YAML should contain 'command:' when command is set:\n%s", yaml)
	}
	// command args must be present
	if !strings.Contains(yaml, "--host") || !strings.Contains(yaml, "0.0.0.0") {
		t.Errorf("YAML should contain command args:\n%s", yaml)
	}
	// vLLM auto-flags must NOT appear (provider is sglang)
	vllmFlags := []string{"--max-model-len", "--gpu-memory-utilization", "--max-num-batched-tokens", "--max-num-seqs"}
	for _, flag := range vllmFlags {
		if strings.Contains(yaml, flag) {
			t.Errorf("YAML should NOT contain vLLM auto-flag %q for sglang provider:\n%s", flag, yaml)
		}
	}
}

// TestCompose_ProviderLlamaCPP_PassThrough verifies that when provider=llama.cpp,
// both entrypoint and command are rendered as-is with NO auto-flags.
func TestCompose_ProviderLlamaCPP_PassThrough(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-llamacpp-passthrough",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test LlamaCPP PassThrough",
		Container: "llm-test-llamacpp",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "ghcr.io/ggerganov/llama.cpp:latest",
		Entrypoint:  []string{"./server"},
		Provider:    "llama.cpp",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{"--host", "0.0.0.0", "--port", "8000"},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// entrypoint: line must appear
	if !strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should contain 'entrypoint:' when entrypoint is set:\n%s", yaml)
	}
	// command: line must appear
	if !strings.Contains(yaml, "command:") {
		t.Errorf("YAML should contain 'command:' when command is set:\n%s", yaml)
	}
	// vLLM auto-flags must NOT appear (provider is llama.cpp)
	vllmFlags := []string{"--max-model-len", "--gpu-memory-utilization", "--max-num-batched-tokens", "--max-num-seqs"}
	for _, flag := range vllmFlags {
		if strings.Contains(yaml, flag) {
			t.Errorf("YAML should NOT contain vLLM auto-flag %q for llama.cpp provider:\n%s", flag, yaml)
		}
	}
}

// TestCompose_EntryPointEmpty_NoRender verifies that when entrypoint is an
// empty slice, the compose YAML does NOT render entrypoint: []. This was Bug 1.
func TestCompose_EntryPointEmpty_NoRender(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-entrypoint-empty",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Entrypoint Empty",
		Container: "llm-test-ep-empty",
		Port:      8080,
	}

	// Entrypoint is explicitly an empty slice (not nil)
	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{},
		Provider:    "custom",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// The key assertion: entrypoint: [] must NOT appear
	if strings.Contains(yaml, "entrypoint: []") {
		t.Errorf("YAML should NOT contain 'entrypoint: []' for empty entrypoint slice:\n%s", yaml)
	}
	// entrypoint: line (even without []) must NOT appear
	lines := strings.Split(yaml, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "entrypoint:") {
			t.Errorf("YAML should NOT contain any 'entrypoint:' directive for empty slice. Line: %q\nFull YAML:\n%s", trimmed, yaml)
		}
	}
}

// TestCompose_EntryPointNonEmpty_Rendered verifies that when entrypoint is
// a non-empty slice, the compose YAML DOES render it with single-quoted items.
func TestCompose_EntryPointNonEmpty_Rendered(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-entrypoint-nonempty",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Entrypoint NonEmpty",
		Container: "llm-test-ep-nonempty",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		Provider:    "vllm",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// entrypoint: line must appear
	if !strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should contain 'entrypoint:' when entrypoint is non-empty:\n%s", yaml)
	}
	// items should be single-quoted
	if !strings.Contains(yaml, "'python3'") {
		t.Errorf("YAML should contain single-quoted 'python3':\n%s", yaml)
	}
}

// TestCompose_CommandEmpty_NoRender verifies that when commandArgs is an
// empty slice, the compose YAML does NOT render command: lines.
func TestCompose_CommandEmpty_NoRender(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-command-empty",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Command Empty",
		Container: "llm-test-cmd-empty",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{"./run.sh"},
		Provider:    "custom",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// command: line must NOT appear
	if strings.Contains(yaml, "command:") {
		t.Errorf("YAML should NOT contain 'command:' when commandArgs is empty:\n%s", yaml)
	}
	// entrypoint: line must appear
	if !strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should contain 'entrypoint:' when entrypoint is non-empty:\n%s", yaml)
	}
}

// TestCompose_ProviderVllm_WithEntrypoint_NoCommand verifies that when
// provider=vllm with a non-empty entrypoint but empty command, only
// entrypoint is rendered (no command: line).
func TestCompose_ProviderVllm_WithEntrypoint_NoCommand(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-vllm-ep-only",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test vLLM EP Only",
		Container: "llm-test-vllm-eponly",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "vllm/vllm-openai:latest",
		Entrypoint:  []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		Provider:    "vllm",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// entrypoint: must appear
	if !strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should contain 'entrypoint:' when entrypoint is set:\n%s", yaml)
	}
	// command: must NOT appear
	if strings.Contains(yaml, "command:") {
		t.Errorf("YAML should NOT contain 'command:' when commandArgs is empty:\n%s", yaml)
	}
}

// TestCompose_EntryPointOnly_NoCommand tests the scenario from the plan:
// entrypoint=["something.sh"], command=[] → only entrypoint rendered.
func TestCompose_EntryPointOnly_NoCommand(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-ep-only-plan",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test EP Only Plan",
		Container: "llm-test-ep-only",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{"something.sh"},
		Provider:    "custom",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if !strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should contain 'entrypoint:' when entrypoint is set:\n%s", yaml)
	}
	if !strings.Contains(yaml, "'something.sh'") {
		t.Errorf("YAML should contain single-quoted 'something.sh':\n%s", yaml)
	}
	if strings.Contains(yaml, "command:") {
		t.Errorf("YAML should NOT contain 'command:' when commandArgs is empty:\n%s", yaml)
	}
}

// TestCompose_CommandOnly_NoEntrypoint tests the scenario from the plan:
// entrypoint=[], command=["run-server.sh arg1"] → only command rendered.
func TestCompose_CommandOnly_NoEntrypoint(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-cmd-only-plan",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test CMD Only Plan",
		Container: "llm-test-cmd-only",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{},
		Provider:    "custom",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{"run-server.sh", "arg1"},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if strings.Contains(yaml, "entrypoint:") {
		t.Errorf("YAML should NOT contain 'entrypoint:' when entrypoint is empty:\n%s", yaml)
	}
	if !strings.Contains(yaml, "command:") {
		t.Errorf("YAML should contain 'command:' when commandArgs is set:\n%s", yaml)
	}
}

// TestMergeProfileFlagsProviderGate tests that mergeProfileFlagsWithOptions
// only modifies commands for the "vllm" provider, returning verbatim for others.
//
// This is tested indirectly through GenerateWithOptions since the function
// is unexported. We use models with TotalParamsB/QuantBytesPerParam set so
// that mergeProfileFlagsWithOptions enters the vLLM-flag-generation path.
func TestMergeProfileFlagsProviderGate(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	params := 7.0
	quant := 1.0

	baseModel := &models.Model{
		Slug:               "test-provider-gate",
		Type:               "llm",
		SubType:            "chat",
		Name:               "Test Provider Gate",
		Container:          "llm-test-gate",
		Port:               8080,
		TotalParamsB:       &params,
		QuantBytesPerParam: &quant,
		DefaultContext:     intPtr(4096),
	}

	tests := []struct {
		name         string
		provider     string
		commandArgs  []string
		wantVllmFlag bool // whether vLLM auto-flags should appear
	}{
		{
			name:         "vllm_provider_modifies_command",
			provider:     "vllm",
			commandArgs:  []string{"--port", "8000"},
			wantVllmFlag: true,
		},
		{
			name:         "custom_provider_unchanged",
			provider:     "custom",
			commandArgs:  []string{"--port", "8000"},
			wantVllmFlag: false,
		},
		{
			name:         "sglang_provider_unchanged",
			provider:     "sglang",
			commandArgs:  []string{"--port", "8000"},
			wantVllmFlag: false,
		},
		{
			name:         "llama.cpp_provider_unchanged",
			provider:     "llama.cpp",
			commandArgs:  []string{"--port", "8000"},
			wantVllmFlag: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := EngineComposeConfig{
				Image:       "myimage:latest",
				Entrypoint:  []string{},
				Provider:    tc.provider,
				EnvVars:     map[string]string{},
				Volumes:     []string{},
				CommandArgs: tc.commandArgs,
			}

			yaml, err := gen.Generate(baseModel, cfg)
			if err != nil {
				t.Fatalf("Generate returned error: %v", err)
			}

			// Check original command args are always present
			if !strings.Contains(yaml, "--port") || !strings.Contains(yaml, "8000") {
				t.Errorf("YAML should contain original command args for provider %q:\n%s", tc.provider, yaml)
			}

			// Check vLLM auto-flags appear only for vllm provider
			vllmFlags := []string{"--max-model-len", "--gpu-memory-utilization", "--max-num-batched-tokens", "--max-num-seqs"}
			for _, flag := range vllmFlags {
				found := strings.Contains(yaml, flag)
				if tc.wantVllmFlag && !found {
					t.Errorf("Provider %q: expected vLLM auto-flag %q to be present, but it was not:\n%s", tc.provider, flag, yaml)
				}
				if !tc.wantVllmFlag && found {
					t.Errorf("Provider %q: expected vLLM auto-flag %q to NOT be present, but it was:\n%s", tc.provider, flag, yaml)
				}
			}
		})
	}
}

// TestMergeProfileFlagsNilModel_NoModification verifies that when model is nil,
// mergeProfileFlagsWithOptions returns existing commands verbatim regardless of provider.
func TestMergeProfileFlagsNilModel_NoModification(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	// Model is nil — mergeProfileFlagsWithOptions should short-circuit
	cfg := EngineComposeConfig{
		Image:       "myimage:latest",
		Entrypoint:  []string{},
		Provider:    "vllm",
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		CommandArgs: []string{"--port", "8000"},
	}

	yaml, err := gen.Generate(nil, cfg)
	if err == nil {
		t.Fatalf("expected error for nil model, got nil")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("error should mention 'model is required': %v", err)
	}
	// The nil model check happens before mergeProfileFlagsWithOptions, so we
	// can't test the nil-model path through Generate directly. Instead, verify
	// that a model without params triggers the short-circuit.
	model := &models.Model{
		Slug:      "test-nil-params",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Nil Params",
		Container: "llm-test-nil-params",
		Port:      8080,
		// TotalParamsB and QuantBytesPerParam are nil
	}

	yaml, err = gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned unexpected error: %v", err)
	}

	// Original command args should be preserved verbatim
	if !strings.Contains(yaml, "--port") || !strings.Contains(yaml, "8000") {
		t.Errorf("YAML should contain original command args:\n%s", yaml)
	}
	// No vLLM auto-flags since model has no params
	vllmFlags := []string{"--max-model-len", "--gpu-memory-utilization"}
	for _, flag := range vllmFlags {
		if strings.Contains(yaml, flag) {
			t.Errorf("YAML should NOT contain vLLM auto-flag %q when model params are nil:\n%s", flag, yaml)
		}
	}
}

// --- Helper functions ---

func intPtr(i int) *int { return &i }
