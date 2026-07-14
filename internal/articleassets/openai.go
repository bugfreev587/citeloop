package articleassets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxImageResponseBytes = 24 << 20

type OpenAIProvider struct {
	APIKey, BaseURL, Model string
	Client                 *http.Client
}

func (p OpenAIProvider) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return GenerateResult{}, errors.New("approved image brief prompt is required")
	}
	if strings.TrimSpace(p.APIKey) == "" || strings.TrimSpace(p.Model) == "" {
		return GenerateResult{}, errors.New("OpenAI image credential and model are required")
	}
	if len(prompt) > 6000 {
		return GenerateResult{}, errors.New("approved image brief prompt is too long")
	}
	role := strings.ReplaceAll(strings.TrimSpace(request.Role), "_", " ")
	prompt = fmt.Sprintf("Create an informative %s image for a factual article. The image must clarify the article, contain no logos or fabricated statistics, and avoid decorative stock-photo styling. Approved brief and outline: %s", role, prompt)
	payload, _ := json.Marshal(map[string]any{"model": strings.TrimSpace(p.Model), "prompt": prompt, "size": "1536x1024", "quality": "medium", "output_format": "png", "n": 1})
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/images/generations", bytes.NewReader(payload))
	if err != nil {
		return GenerateResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.APIKey))
	req.Header.Set("Content-Type", "application/json")
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("OpenAI image request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxImageResponseBytes+1))
	if err != nil {
		return GenerateResult{}, err
	}
	if len(raw) > maxImageResponseBytes {
		return GenerateResult{}, errors.New("OpenAI image response exceeded the size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if len(message) > 512 {
			message = message[:512]
		}
		return GenerateResult{}, fmt.Errorf("OpenAI image request returned %d: %s", resp.StatusCode, message)
	}
	var output struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &output); err != nil || len(output.Data) == 0 || output.Data[0].B64JSON == "" {
		return GenerateResult{}, errors.New("OpenAI image response contained no image data")
	}
	image, err := base64.StdEncoding.DecodeString(output.Data[0].B64JSON)
	if err != nil {
		return GenerateResult{}, errors.New("OpenAI image response was not valid base64")
	}
	return GenerateResult{Bytes: image, MimeType: "image/png", Provider: "openai", Model: strings.TrimSpace(p.Model), Width: 1536, Height: 1024}, nil
}
