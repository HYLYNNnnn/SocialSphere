package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type ImageError struct {
	Status  int
	Message string
}

func (e ImageError) Error() string { return e.Message }

type ImageGenerator interface {
	Generate(context.Context, string, string) (string, error)
	Enabled() bool
}

type OpenAIImageGenerator struct {
	key      string
	model    string
	endpoint string
	client   *http.Client
}

func NewOpenAIImageGenerator(key, model string, client *http.Client, endpoint string) *OpenAIImageGenerator {
	if client == nil {
		client = &http.Client{Timeout: 150 * time.Second}
	}
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/images/generations"
	}
	return &OpenAIImageGenerator{key: key, model: model, client: client, endpoint: endpoint}
}
func (g *OpenAIImageGenerator) Enabled() bool { return strings.TrimSpace(g.key) != "" }

func (g *OpenAIImageGenerator) Generate(ctx context.Context, username, prompt string) (string, error) {
	if !g.Enabled() {
		return "", ImageError{503, "AI image generation is not configured on the server."}
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || !utf8.ValidString(prompt) || utf8.RuneCountInString(prompt) > 2000 {
		return "", ValidationError("Describe an image using 1–2000 characters.")
	}
	identity := sha256.Sum256([]byte(username))
	payload := map[string]interface{}{
		"model": g.model, "prompt": prompt, "n": 1, "size": "1024x1024",
		"quality": "low", "output_format": "png", "safety_identifier": hex.EncodeToString(identity[:]),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+g.key)
	response, err := g.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", ImageError{504, "Image generation timed out. Please try again."}
		}
		return "", ImageError{502, "The image service could not be reached."}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return "", ImageError{429, "The image service is busy. Please try again later."}
	}
	if response.StatusCode == 400 {
		return "", ImageError{422, "The image request was rejected. Try a different description."}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", ImageError{502, "Image generation is temporarily unavailable. Check the server configuration."}
	}
	const maxResponseBytes = 16 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return "", ImageError{502, "The image service returned an invalid response."}
	}
	var result struct {
		Data []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Data) != 1 || result.Data[0].B64 == "" {
		return "", ImageError{502, "The image service did not return an image."}
	}
	imageData, err := base64.StdEncoding.DecodeString(result.Data[0].B64)
	if err != nil {
		return "", ImageError{502, "The image service returned an invalid image."}
	}
	kind, contentType, err := ValidateMedia(imageData)
	if err != nil || kind != "image" {
		return "", ImageError{502, "The generated image is invalid or too large to upload."}
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(imageData), nil
}
