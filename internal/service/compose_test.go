package service

import (
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
)

func TestNewComposeGenerator_ComfyUITemplateParses(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}
	if gen == nil {
		t.Fatal("NewComposeGenerator returned nil")
	}
	if gen.comfyUITmpl == nil {
		t.Fatal("ComfyUI template was not parsed")
	}
}

func TestGenerateComfyUICompose_ValidData(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	yaml, err := gen.GenerateComfyUICompose(ComfyUIComposeTemplateData{
		ImageName:     "comfyanonymous/ComfyUI",
		ImageTag:      "latest",
		HostPort:      8188,
		VolumePath:    "/opt/ai-server/comfyui-models",
		ContainerName: "comfyui-flux",
	})
	if err != nil {
		t.Fatalf("GenerateComfyUICompose returned error: %v", err)
	}

	// Verify key fields are rendered
	checks := []struct {
		name   string
		needle string
	}{
		{"image", "image: comfyanonymous/ComfyUI:latest"},
		{"container_name", "container_name: comfyui-flux"},
		{"host port", "8188:8188"},
		{"volume", "/opt/ai-server/comfyui-models:/home/runner/ComfyUI/models"},
		{"environment", "CLI_ARGS=--listen 0.0.0.0"},
		{"restart", "restart: unless-stopped"},
		{"gpu runtime", "runtime: nvidia"},
		{"gpu driver", "driver: nvidia"},
		{"gpu count", "count: all"},
		{"gpu capabilities", "capabilities: [gpu]"},
		{"merge key", "<<: *gpu-service"},
		{"x-gpu-service anchor", "x-gpu-service: &gpu-service"},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(yaml, tc.needle) {
				t.Errorf("YAML missing %q:\n%s", tc.needle, yaml)
			}
		})
	}
}

func TestGenerateComfyUICompose_MissingImageName(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	_, err = gen.GenerateComfyUICompose(ComfyUIComposeTemplateData{
		ImageTag:      "latest",
		HostPort:      8188,
		VolumePath:    "/opt/ai-server/comfyui-models",
		ContainerName: "comfyui-flux",
	})
	if err == nil {
		t.Error("expected error for missing ImageName, got nil")
	}
	if !strings.Contains(err.Error(), "ImageName") {
		t.Errorf("error should mention ImageName: %v", err)
	}
}

func TestGenerateComfyUICompose_MissingContainerName(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	_, err = gen.GenerateComfyUICompose(ComfyUIComposeTemplateData{
		ImageName:     "comfyanonymous/ComfyUI",
		ImageTag:      "latest",
		HostPort:      8188,
		VolumePath:    "/opt/ai-server/comfyui-models",
		ContainerName: "",
	})
	if err == nil {
		t.Error("expected error for missing ContainerName, got nil")
	}
	if !strings.Contains(err.Error(), "ContainerName") {
		t.Errorf("error should mention ContainerName: %v", err)
	}
}

func TestGenerateComfyUICompose_CustomPort(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	yaml, err := gen.GenerateComfyUICompose(ComfyUIComposeTemplateData{
		ImageName:     "comfyanonymous/ComfyUI",
		ImageTag:      "master",
		HostPort:      9000,
		VolumePath:    "/data/comfyui",
		ContainerName: "comfyui-custom",
	})
	if err != nil {
		t.Fatalf("GenerateComfyUICompose returned error: %v", err)
	}

	expected := "9000:8188"
	if !strings.Contains(yaml, expected) {
		t.Errorf("YAML should contain port mapping %q, got:\n%s", expected, yaml)
	}
}

func TestGenerateComfyUICompose_GpuServiceStructure(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	yaml, err := gen.GenerateComfyUICompose(ComfyUIComposeTemplateData{
		ImageName:     "comfyanonymous/ComfyUI",
		ImageTag:      "latest",
		HostPort:      8188,
		VolumePath:    "/opt/ai-server/comfyui-models",
		ContainerName: "comfyui-flux",
	})
	if err != nil {
		t.Fatalf("GenerateComfyUICompose returned error: %v", err)
	}

	// Verify the deploy section structure
	deployChecks := []string{
		"deploy:",
		"resources:",
		"reservations:",
		"devices:",
	}
	for _, needle := range deployChecks {
		if !strings.Contains(yaml, needle) {
			t.Errorf("YAML missing deploy structure %q:\n%s", needle, yaml)
		}
	}
}

func TestHealthCheck_ChatModel_IncludesHealthcheck(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-chat-model",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Chat Model",
		Container: "llm-test-chat",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:      "vllm/vllm-openai:latest",
		Entrypoint: []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:    map[string]string{},
		Volumes:    []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify healthcheck section is present
	healthcheckChecks := []struct {
		name   string
		needle string
	}{
		{"healthcheck key", "healthcheck:"},
		{"test command", `test: ["CMD", "curl", "-f", "http://localhost:8000/health"]`},
		{"interval", "interval: 30s"},
		{"timeout", "timeout: 10s"},
		{"retries", "retries: 10"},
		{"start_period", "start_period: 180s"},
	}

	for _, tc := range healthcheckChecks {
		if !strings.Contains(yaml, tc.needle) {
			t.Errorf("YAML missing %q:\n%s", tc.needle, yaml)
		}
	}
}

func TestHealthCheck_NonChatModel_NoHealthcheck(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	// SubType is empty — should NOT include healthcheck
	model := &models.Model{
		Slug:      "test-embedding-model",
		Type:      "llm",
		SubType:   "embedding",
		Name:      "Test Embedding Model",
		Container: "llm-test-embed",
		Port:      8081,
	}

	cfg := EngineComposeConfig{
		Image:      "vllm/vllm-openai:latest",
		Entrypoint: []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:    map[string]string{},
		Volumes:    []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if strings.Contains(yaml, "healthcheck:") {
		t.Errorf("non-chat model YAML should not contain healthcheck:\n%s", yaml)
	}
}

func TestHealthCheck_NonLLMType_NoHealthcheck(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	// Type is not "llm" — should NOT include healthcheck even if SubType is "chat"
	model := &models.Model{
		Slug:      "test-rag-model",
		Type:      "rag",
		SubType:   "chat",
		Name:      "Test RAG Model",
		Container: "rag-test",
		Port:      8082,
	}

	cfg := EngineComposeConfig{
		Image:      "some/image:latest",
		Entrypoint: []string{"/entrypoint.sh"},
		EnvVars:    map[string]string{},
		Volumes:    []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if strings.Contains(yaml, "healthcheck:") {
		t.Errorf("non-LLM type YAML should not contain healthcheck:\n%s", yaml)
	}
}

func TestGenerateComfyUICompose_TableDriven(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	tests := []struct {
		name     string
		data     ComfyUIComposeTemplateData
		wantErr  bool
		checks   []string
		notCheck []string
	}{
		{
			name: "default latest tag",
			data: ComfyUIComposeTemplateData{
				ImageName:     "comfyanonymous/ComfyUI",
				ImageTag:      "latest",
				HostPort:      8188,
				VolumePath:    "/opt/ai-server/comfyui-models",
				ContainerName: "comfyui-flux",
			},
			wantErr: false,
			checks: []string{
				"image: comfyanonymous/ComfyUI:latest",
				"container_name: comfyui-flux",
			},
		},
		{
			name: "custom tag and port",
			data: ComfyUIComposeTemplateData{
				ImageName:     "comfyanonymous/ComfyUI",
				ImageTag:      "v1.2.3",
				HostPort:      3000,
				VolumePath:    "/mnt/models",
				ContainerName: "comfyui-v1",
			},
			wantErr: false,
			checks: []string{
				"image: comfyanonymous/ComfyUI:v1.2.3",
				"container_name: comfyui-v1",
				"3000:8188",
			},
		},
		{
			name: "empty image name error",
			data: ComfyUIComposeTemplateData{
				ImageTag:      "latest",
				HostPort:      8188,
				VolumePath:    "/opt/models",
				ContainerName: "comfyui",
			},
			wantErr:  true,
			checks:   nil,
			notCheck: nil,
		},
		{
			name: "empty container name error",
			data: ComfyUIComposeTemplateData{
				ImageName:     "comfyanonymous/ComfyUI",
				ImageTag:      "latest",
				HostPort:      8188,
				VolumePath:    "/opt/models",
				ContainerName: "",
			},
			wantErr:  true,
			checks:   nil,
			notCheck: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml, err := gen.GenerateComfyUICompose(tc.data)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, needle := range tc.checks {
				if !strings.Contains(yaml, needle) {
					t.Errorf("YAML missing %q:\n%s", needle, yaml)
				}
			}
			for _, notNeedle := range tc.notCheck {
				if strings.Contains(yaml, notNeedle) {
					t.Errorf("YAML should not contain %q:\n%s", notNeedle, yaml)
				}
			}
		})
	}
}

// TestHealthCheck_Custom_OverridesAutoInjected verifies that when an engine
// version provides a custom HealthcheckJSON on a chat-type LLM model, the
// custom healthcheck is rendered instead of the auto-injected default.
func TestHealthCheck_Custom_OverridesAutoInjected(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-chat-model",
		Type:      "llm",
		SubType:   "chat",
		Name:      "Test Chat Model",
		Container: "llm-test-chat",
		Port:      8080,
	}

	// Custom healthcheck YAML block (different from auto-injected default)
	customHC := `    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://localhost:8000/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 240s`

	cfg := EngineComposeConfig{
		Image:              "vllm/vllm-openai:latest",
		Entrypoint:         []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:            map[string]string{},
		Volumes:            []string{},
		HealthCheckSection: customHC, // simulates custom healthcheck passed in
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Custom healthcheck should be present
	if !strings.Contains(yaml, `test: ["CMD", "curl", "-fsS", "http://localhost:8000/health"]`) {
		t.Errorf("YAML should contain custom healthcheck test command:\n%s", yaml)
	}
	if !strings.Contains(yaml, "timeout: 5s") {
		t.Errorf("YAML should contain custom timeout 5s:\n%s", yaml)
	}
	if !strings.Contains(yaml, "retries: 3") {
		t.Errorf("YAML should contain custom retries 3:\n%s", yaml)
	}

	// Auto-injected defaults should NOT be present
	if strings.Contains(yaml, `test: ["CMD", "curl", "-f", "http://localhost:8000/health"]`) {
		t.Errorf("YAML should NOT contain auto-injected healthcheck:\n%s", yaml)
	}
	if strings.Contains(yaml, "retries: 10") {
		t.Errorf("YAML should NOT contain auto-injected retries 10:\n%s", yaml)
	}
}

// TestUlimits_Rendered verifies that when an engine version provides UlimitsJSON,
// a properly formatted ulimits block appears in the compose YAML.
func TestUlimits_Rendered(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-llm-model",
		Type:      "llm",
		SubType:   "completion",
		Name:      "Test LLM Model",
		Container: "llm-test",
		Port:      8080,
	}

	// Pre-rendered ulimits YAML block
	ulimitsBlock := `    ulimits:
      memlock: -1
      stack: 67108864`

	cfg := EngineComposeConfig{
		Image:          "vllm/vllm-openai:latest",
		Entrypoint:     []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:        map[string]string{},
		Volumes:        []string{},
		UlimitsSection: ulimitsBlock,
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify ulimits block is present with correct values
	if !strings.Contains(yaml, "ulimits:") {
		t.Errorf("YAML should contain ulimits block:\n%s", yaml)
	}
	if !strings.Contains(yaml, "memlock: -1") {
		t.Errorf("YAML should contain memlock: -1:\n%s", yaml)
	}
	if !strings.Contains(yaml, "stack: 67108864") {
		t.Errorf("YAML should contain stack: 67108864:\n%s", yaml)
	}
}

// TestIPC_Override verifies that when an engine version provides a non-empty
// IPC value, the compose YAML renders that value instead of the default "host".
func TestIPC_Override(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-llm-model",
		Type:      "llm",
		SubType:   "completion",
		Name:      "Test LLM Model",
		Container: "llm-test",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "vllm/vllm-openai:latest",
		Entrypoint:  []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		IPCOverride: "share",
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if !strings.Contains(yaml, "ipc: share") {
		t.Errorf("YAML should contain 'ipc: share':\n%s", yaml)
	}
}

// TestIPC_Empty_DefaultsToHost verifies that when an engine version has an
// empty IPC field, the compose YAML renders the default "ipc: host".
func TestIPC_Empty_DefaultsToHost(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-llm-model",
		Type:      "llm",
		SubType:   "completion",
		Name:      "Test LLM Model",
		Container: "llm-test",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:       "vllm/vllm-openai:latest",
		Entrypoint:  []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:     map[string]string{},
		Volumes:     []string{},
		IPCOverride: "",
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if !strings.Contains(yaml, "ipc: host") {
		t.Errorf("YAML should contain default 'ipc: host':\n%s", yaml)
	}
}

// TestUlimits_Empty_NoRender verifies that when an engine version has no
// ulimits configured, the compose YAML does not contain an ulimits block.
func TestUlimits_Empty_NoRender(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-llm-model",
		Type:      "llm",
		SubType:   "completion",
		Name:      "Test LLM Model",
		Container: "llm-test",
		Port:      8080,
	}

	cfg := EngineComposeConfig{
		Image:          "vllm/vllm-openai:latest",
		Entrypoint:     []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:        map[string]string{},
		Volumes:        []string{},
		UlimitsSection: "",
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if strings.Contains(yaml, "ulimits:") {
		t.Errorf("YAML should NOT contain ulimits block when empty:\n%s", yaml)
	}
}

// TestHealthCheck_Model_OverridesEngine verifies that when a model has a
// non-empty HealthcheckJSON, the model-level healthcheck takes priority over
// the engine-level healthcheck (passed in cfg.HealthCheckSection).
func TestHealthCheck_Model_OverridesEngine(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:            "test-chat-model",
		Type:            "llm",
		SubType:         "chat",
		Name:            "Test Chat Model",
		Container:       "llm-test-chat",
		Port:            8080,
		HealthcheckJSON: `{"test": ["CMD-SHELL", "curl -f http://localhost:8000/health || exit 1"], "interval": "15s", "timeout": "3s", "retries": 5, "start_period": "300s"}`,
	}

	// Engine-level healthcheck (should be overridden by model)
	engineHC := `    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
      interval: 30s
      timeout: 10s
      retries: 10
      start_period: 180s`

	cfg := EngineComposeConfig{
		Image:                "vllm/vllm-openai:latest",
		Entrypoint:           []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:              map[string]string{},
		Volumes:              []string{},
		HealthCheckSection:   engineHC, // engine healthcheck
		ModelHealthcheckJSON: `{"test": ["CMD-SHELL", "curl -f http://localhost:8000/health || exit 1"], "interval": "15s", "timeout": "3s", "retries": 5, "start_period": "300s"}`,
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Model-level healthcheck should be present
	if !strings.Contains(yaml, `test: ["CMD-SHELL", "curl -f http://localhost:8000/health || exit 1"]`) {
		t.Errorf("YAML should contain model-level healthcheck test command:\n%s", yaml)
	}
	if !strings.Contains(yaml, `interval: "15s"`) {
		t.Errorf("YAML should contain model-level interval 15s:\n%s", yaml)
	}
	if !strings.Contains(yaml, `timeout: "3s"`) {
		t.Errorf("YAML should contain model-level timeout 3s:\n%s", yaml)
	}
	if !strings.Contains(yaml, "retries: 5") {
		t.Errorf("YAML should contain model-level retries 5:\n%s", yaml)
	}
	if !strings.Contains(yaml, `start_period: "300s"`) {
		t.Errorf("YAML should contain model-level start_period 300s:\n%s", yaml)
	}

	// Engine-level healthcheck values should NOT be present
	if strings.Contains(yaml, "retries: 10") {
		t.Errorf("YAML should NOT contain engine-level retries 10:\n%s", yaml)
	}
	if strings.Contains(yaml, "start_period: 180s") {
		t.Errorf("YAML should NOT contain engine-level start_period 180s:\n%s", yaml)
	}
}

// TestHealthCheck_Engine_UsedWhenModelHasNone verifies that when a model has no
// HealthcheckJSON configured, the engine-level healthcheck is used.
func TestHealthCheck_Engine_UsedWhenModelHasNone(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	// Model with no healthcheck
	model := &models.Model{
		Slug:            "test-chat-model",
		Type:            "llm",
		SubType:         "chat",
		Name:            "Test Chat Model",
		Container:       "llm-test-chat",
		Port:            8080,
		HealthcheckJSON: "",
	}

	// Engine-level custom healthcheck
	engineHC := `    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://localhost:8000/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 240s`

	cfg := EngineComposeConfig{
		Image:              "vllm/vllm-openai:latest",
		Entrypoint:         []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:            map[string]string{},
		Volumes:            []string{},
		HealthCheckSection: engineHC,
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Engine healthcheck should be present
	if !strings.Contains(yaml, `test: ["CMD", "curl", "-fsS", "http://localhost:8000/health"]`) {
		t.Errorf("YAML should contain engine-level healthcheck:\n%s", yaml)
	}
	if !strings.Contains(yaml, "timeout: 5s") {
		t.Errorf("YAML should contain engine-level timeout 5s:\n%s", yaml)
	}

	// Auto-injected defaults should NOT be present
	if strings.Contains(yaml, "retries: 10") {
		t.Errorf("YAML should NOT contain auto-injected retries 10:\n%s", yaml)
	}
}

// TestHealthCheck_AutoInjected_WhenNeitherModelNorEngineHasOne verifies that when
// neither model nor engine has a healthcheck configured, the auto-injected default
// is used for chat-type LLM models.
func TestHealthCheck_AutoInjected_WhenNeitherModelNorEngineHasOne(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	// Chat model with no model or engine healthcheck
	model := &models.Model{
		Slug:            "test-chat-model",
		Type:            "llm",
		SubType:         "chat",
		Name:            "Test Chat Model",
		Container:       "llm-test-chat",
		Port:            8080,
		HealthcheckJSON: "",
	}

	cfg := EngineComposeConfig{
		Image:              "vllm/vllm-openai:latest",
		Entrypoint:         []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:            map[string]string{},
		Volumes:            []string{},
		HealthCheckSection: "",
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Auto-injected healthcheck should be present
	if !strings.Contains(yaml, "healthcheck:") {
		t.Errorf("YAML should contain auto-injected healthcheck:\n%s", yaml)
	}
	if !strings.Contains(yaml, `test: ["CMD", "curl", "-f", "http://localhost:8000/health"]`) {
		t.Errorf("YAML should contain auto-injected test command:\n%s", yaml)
	}
	if !strings.Contains(yaml, "retries: 10") {
		t.Errorf("YAML should contain auto-injected retries 10:\n%s", yaml)
	}
}

// TestHealthCheck_NonChatModel_NoAutoInject verifies that non-chat models do not
// get auto-injected healthcheck when neither model nor engine has one.
func TestHealthCheck_NonChatModel_NoAutoInject(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	// Embedding model (not chat) with no model or engine healthcheck
	model := &models.Model{
		Slug:            "test-embed-model",
		Type:            "llm",
		SubType:         "embedding",
		Name:            "Test Embed Model",
		Container:       "llm-test-embed",
		Port:            8081,
		HealthcheckJSON: "",
	}

	cfg := EngineComposeConfig{
		Image:              "vllm/vllm-openai:latest",
		Entrypoint:         []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:            map[string]string{},
		Volumes:            []string{},
		HealthCheckSection: "",
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if strings.Contains(yaml, "healthcheck:") {
		t.Errorf("non-chat model YAML should not contain healthcheck:\n%s", yaml)
	}
}

// TestEntrypoint_UsesSingleQuotes verifies that the compose template renders
// entrypoint items with single-quoted strings (not escaped double-quotes),
// fixing Bug 1 where \\" rendered literal backslash-quote sequences that
// Docker Compose could not parse as valid inline array syntax.
func TestEntrypoint_UsesSingleQuotes(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:      "test-model",
		Type:      "speech",
		SubType:   "tts",
		Name:      "Test TTS Model",
		Container: "speech-test-tts",
		Port:      8004,
	}

	cfg := EngineComposeConfig{
		Image:      "myimage:v1",
		Entrypoint: []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
		EnvVars:    map[string]string{},
		Volumes:    []string{},
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Should contain single-quoted entries like 'python3'
	if !strings.Contains(yaml, "'python3'") {
		t.Errorf("entrypoint should use single-quoted entries:\n%s", yaml)
	}
	// Should NOT contain escaped double-quotes like \"python3\"
	if strings.Contains(yaml, `\\"python3\\"`) || strings.Contains(yaml, `\\\"python3\\\"`) {
		t.Errorf("entrypoint should NOT contain escaped quote sequences:\n%s", yaml)
	}
	// Entry line pattern: entrypoint: ['python3', '-m', ...]
	lines := strings.Split(yaml, "\n")
	var entryFound bool
	for i, line := range lines {
		if strings.Contains(line, "entrypoint: [") {
			entryFound = true
			// Check first item is single-quoted
			if !strings.Contains(line, "'python3'") {
				t.Errorf("entryline at line %d should contain single-quoted 'python3':\n%s", i+1, line)
			}
		}
	}
	if !entryFound {
		t.Error("composeline should contain an entrypoint directive")
	}
}

// TestHealthCheckJSON_InGeneratedComposes verifies that model-level healthcheck
// JSON is correctly rendered as a YAML healthcheck block in the generated compose
// output (Bug 2 — importing healthcheck from YAML into DB and re-rendering).
func TestHealthCheckJSON_InGeneratedComposes(t *testing.T) {
	gen, err := NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator returned error: %v", err)
	}

	model := &models.Model{
		Slug:            "tts-health-model",
		Type:            "speech",
		SubType:         "tts",
		Name:            "TTS Health Model",
		Container:       "speech-tts-health",
		Port:            8004,
		HealthcheckJSON: `{"test": ["CMD", "curl", "-fsS", "http://localhost:8000/v1/models"], "interval": "30s", "timeout": "5s", "retries": 3, "start_period": "600s"}`,
	}

	cfg := EngineComposeConfig{
		Image:                "myimage:v1",
		Entrypoint:           []string{"python3", "-m", "app"},
		EnvVars:              map[string]string{},
		Volumes:              []string{},
		HealthCheckSection:   "",
		ModelHealthcheckJSON: model.HealthcheckJSON,
	}

	yaml, err := gen.Generate(model, cfg)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify healthcheck section is present
	if !strings.Contains(yaml, "healthcheck:") {
		t.Fatalf("Generated YAML should contain healthcheck:\\n%s", yaml)
	}
	// Verify all key fields from the JSON
	expectedFields := []string{
		`test: ["CMD", "curl", "-fsS", "http://localhost:8000/v1/models"]`,
		`interval: "30s"`,
		`timeout: "5s"`,
		`retries: 3`,
		`start_period: "600s"`,
	}
	for _, f := range expectedFields {
		if !strings.Contains(yaml, f) {
			t.Errorf("YAML missing expected healthcheck field %q:\n%s", f, yaml)
		}
	}
	// Port mapping should render as single-quoted string
	if !strings.Contains(yaml, "\x278004:8000\x27") {
		t.Fatalf("port mapping should be \x278004:8000\x27, got:\n%s", yaml)
	}

	t.Logf("healthcheck compose output:\n%s", yaml)
}
