package service

import (
	"strings"
	"testing"
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
