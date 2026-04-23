// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <LITELLM_BASE_URL> <API_KEY>\n", os.Args[0])
		os.Exit(1)
	}
	baseURL := strings.TrimRight(os.Args[1], "/")
	apiKey := os.Args[2]
	slug := "qwen3-6-nvfp4"

	c := &http.Client{Timeout: 30 * time.Second}

	fmt.Println("===========================================")
	fmt.Printf("Diagnostic for slug: %s\n", slug)
	fmt.Printf("Base URL: %s\n", baseURL)
	fmt.Println("===========================================\n")

	// Test A: GET /models (OpenAI format)
	fmt.Println("--- Test A: GET /models ---")
	testGet(c, baseURL+"/models", apiKey)

	// Test B: Current broken approach
	fmt.Println("\n--- Test B: GET /model/info?" + slug)
	testGet(c, baseURL+"/model/info?"+slug, apiKey)

	// Test C: GET /model/info without filter -> ALL models
	fmt.Println("\n--- Test C: GET /model/info (no params - ALL models) ---")
	testAndParseAllModels(c, baseURL+"/model/info", apiKey, slug)

	// Test D: GET /v1/model/info variant
	fmt.Println("\n--- Test D: GET /v1/model/info?" + slug)
	testGet(c, baseURL+"/v1/model/info?"+slug, apiKey)

	// Test E: check what keys exist in model_info of each returned entry
	fmt.Println("\n--- Check: model_name field format vs slug lookup ---")
	checkModelNameFormat(c, baseURL+"/model/info", apiKey, slug)
}

type liteLLMModelInfo struct {
	ModelName     string                 `json:"model_name"`
	LiteLLMParams map[string]interface{} `json:"litellm_params"`
	ModelInfo     map[string]interface{} `json:"model_info"`
}

func testGet(c *http.Client, fullURL, apiKey string) {
	req, _ := http.NewRequest("GET", fullURL, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := c.Do(req)
	if err != nil {
		fmt.Printf("ERROR: %v\n\n", err)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	fmt.Printf("HTTP %d | bytes=%d\n%s\n\n", resp.StatusCode, len(raw), truncate(string(raw), 800))
}

func testAndParseAllModels(c *http.Client, url, apiKey, targetSlug string) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := c.Do(req)
	if err != nil {
		fmt.Printf("ERROR: %v\n\n", err)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	fmt.Printf("HTTP %d | total_bytes=%d\n", resp.StatusCode, len(raw))

	var result struct {
		ModelInfo map[string]liteLLMModelInfo `json:"model_info"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		fmt.Printf("JSON parse error: %v\n\n", err)
		return
	}

	fmt.Printf("Total models in response: %d\n", len(result.ModelInfo))
	
	found := false
	for key, info := range result.ModelInfo {
		id := ""
		if mi, ok := info.ModelInfo["id"]; ok {
			id = fmt.Sprintf("%v", mi)
		}
		name := info.ModelName
		keyPreview := key
		if len(keyPreview) > 50 {
			keyPreview = keyPreview[:50] + "..."
		}
		fmt.Printf("  key=%s | name=%s | model_info.id=%s\n", 
			keyPreview, name, id)
		
		if containsIgnoreCase(key, targetSlug) || containsIgnoreCase(name, targetSlug) {
			found = true
			fmt.Printf("  >>> MATCH FOR '%s' <<<\n", targetSlug)
			fmt.Printf("    Full key: %s\n", key)
			fmt.Printf("    model_name: %s\n", name)
			fmt.Printf("    model_info.id: %s\n", id)
		}
	}
	if !found {
		fmt.Printf("  >>> NO KEY MATCHED '%s' <<<\n", targetSlug)
		allKeys := make([]string, 0, len(result.ModelInfo))
		i := 0
		for k := range result.ModelInfo {
			if i >= 10 { break }
			allKeys = append(allKeys, k)
			i++
		}
		fmt.Printf("  Sample keys (%d total): %v\n", len(result.ModelInfo), allKeys)
	}
	fmt.Println()
}

func checkModelNameFormat(c *http.Client, url, apiKey, targetSlug string) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	
	var result struct {
		ModelInfo map[string]liteLLMModelInfo `json:"model_info"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return
	}
	
	for _, info := range result.ModelInfo {
		fmt.Printf("  model_name value: %q \n", info.ModelName)
		fmt.Printf("  model_info keys: ")
		for k := range info.ModelInfo {
			fmt.Printf("%s ", k)
		}
		fmt.Println()
		break // just first one
	}
}

func containsIgnoreCase(a, b string) bool {
	return strings.Contains(strings.ToLower(a), strings.ToLower(b))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... (truncated)"
}
