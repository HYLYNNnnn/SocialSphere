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

const DefaultImageModel = "gpt-image-2.5-sunburst"

type ImageModel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var imageModels = [...]ImageModel{
	{ID: "gpt-image-2", Label: "GPT Image 2"},
	{ID: "gpt-image-2.5-sunburst", Label: "GPT Image 2.5 Sunburst"},
	{ID: "gpt-image-2.5-flare", Label: "GPT Image 2.5 Flare"},
	{ID: "gpt-image-2.6-sunburst", Label: "GPT Image 2.6 Sunburst"},
	{ID: "gpt-image-2.6-flare", Label: "GPT Image 2.6 Flare"},
}

func SupportedImageModels() []ImageModel {
	return append([]ImageModel(nil), imageModels[:]...)
}

func IsSupportedImageModel(model string) bool {
	for _, candidate := range imageModels {
		if candidate.ID == model {
			return true
		}
	}
	return false
}

// These options belong to one request. Never save them in a shared generator.
type ImageGenerationOptions struct {
	Model          string
	UsePersonalKey bool
	APIKey         string
}

type ConfigurableImageGenerator interface {
	ImageGenerator
	GenerateWithOptions(context.Context, string, string, ImageGenerationOptions) (string, error)
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
	// Do not forward a server or personal key through an upstream redirect.
	requestClient := *client
	requestClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/images/generations"
	}
	return &OpenAIImageGenerator{key: key, model: model, client: &requestClient, endpoint: endpoint}
}
func (g *OpenAIImageGenerator) Enabled() bool { return strings.TrimSpace(g.key) != "" }

func (g *OpenAIImageGenerator) GenerateWithOptions(ctx context.Context, username, prompt string, options ImageGenerationOptions) (string, error) {
	requestGenerator := *g
	if options.Model != "" {
		requestGenerator.model = options.Model
	}
	if options.UsePersonalKey {
		requestGenerator.key = options.APIKey
		if strings.TrimSpace(requestGenerator.key) == "" {
			return "", ValidationError("A personal OpenAI API key is required.")
		}
	}
	return requestGenerator.Generate(ctx, username, prompt)
}

func (g *OpenAIImageGenerator) Generate(ctx context.Context, username, prompt string) (string, error) {
	if !IsSupportedImageModel(g.model) {
		return "", ValidationError("Select a supported image model.")
	}
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
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", imageProviderError(response)
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

func imageProviderError(response *http.Response) ImageError {
	// Interpret only known codes; provider text may contain private request details.
	var body struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&body)
	if response.StatusCode == http.StatusUnauthorized {
		return ImageError{502, "The image provider rejected the API key. Check the selected key and try again."}
	}
	if response.StatusCode == http.StatusForbidden {
		return ImageError{502, "The selected API key does not have access to this image model. Check the key and model."}
	}
	if response.StatusCode == http.StatusPaymentRequired || body.Error.Code == "insufficient_quota" ||
		body.Error.Code == "billing_hard_limit_reached" || body.Error.Type == "insufficient_quota" {
		return ImageError{402, "The image account has insufficient quota or a billing limit. Check the selected account's billing settings."}
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return ImageError{429, "The image provider rate limit was reached. Please wait and try again."}
	}
	if response.StatusCode == http.StatusBadRequest {
		return ImageError{422, "The image request was rejected. Check model access or try a different description."}
	}
	return ImageError{502, "Image generation is temporarily unavailable. Check the selected key and model."}
}
