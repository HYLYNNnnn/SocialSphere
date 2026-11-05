package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestImageGenerationUsesOnlyServerKeyAndCurrentImageSchema(t *testing.T) {
	image := fixturePNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer server-test-key" {
			t.Error("server key not used")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "gpt-image-2.5-sunburst" || body["output_format"] != "png" || body["n"] != float64(1) || body["prompt"] != "A photo" {
			t.Error("unexpected image API request")
		}
		if _, exists := body["response_format"]; exists {
			t.Error("retired response format used")
		}
		if body["safety_identifier"] == "alice" || len(body["safety_identifier"].(string)) != 64 {
			t.Error("unhashed identity")
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(image)}}})
	}))
	defer upstream.Close()
	client := NewOpenAIImageGenerator("server-test-key", "gpt-image-2.5-sunburst", upstream.Client(), upstream.URL)
	url, err := client.Generate(context.Background(), "alice", " A photo ")
	if err != nil || url != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(image) {
		t.Fatal("generation failed", err)
	}
}

func TestPersonalImageRequestsKeepKeysAndModelsIsolated(t *testing.T) {
	image := fixturePNG(t)
	var received sync.Map
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		received.Store(body["prompt"], r.Header.Get("Authorization")+"|"+body["model"].(string))
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(image)}}})
	}))
	defer upstream.Close()
	client := NewOpenAIImageGenerator("fake-server-key", DefaultImageModel, upstream.Client(), upstream.URL)
	var requests sync.WaitGroup
	for _, tc := range []struct{ user, key, model string }{
		{"alice", "fake-alice-key", "gpt-image-2"},
		{"bob", "fake-bob-key", "gpt-image-2.6-flare"},
	} {
		requests.Add(1)
		go func() {
			defer requests.Done()
			_, err := client.GenerateWithOptions(context.Background(), tc.user, tc.user, ImageGenerationOptions{
				Model: tc.model, UsePersonalKey: true, APIKey: tc.key,
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	requests.Wait()
	if _, err := client.Generate(context.Background(), "server", "server"); err != nil {
		t.Fatal(err)
	}
	for prompt, want := range map[string]string{
		"alice":  "Bearer fake-alice-key|gpt-image-2",
		"bob":    "Bearer fake-bob-key|gpt-image-2.6-flare",
		"server": "Bearer fake-server-key|" + DefaultImageModel,
	} {
		if got, ok := received.Load(prompt); !ok || got != want {
			t.Errorf("request %s used another request's credentials or model", prompt)
		}
	}
	if client.key != "fake-server-key" || client.model != DefaultImageModel {
		t.Fatal("shared provider was mutated")
	}
	if _, err := client.GenerateWithOptions(context.Background(), "alice", "empty personal", ImageGenerationOptions{UsePersonalKey: true}); err == nil {
		t.Fatal("empty personal key fell back to the server")
	}
}

func TestImageProviderQuotaErrorsAndRedirectsDoNotExposeSecrets(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"insufficient_quota","message":"fake-private-key provider-detail"}}`))
	}))
	client := NewOpenAIImageGenerator("fake-private-key", DefaultImageModel, upstream.Client(), upstream.URL)
	_, err := client.Generate(context.Background(), "alice", "A photo")
	var failure ImageError
	if !errors.As(err, &failure) || failure.Status != http.StatusPaymentRequired ||
		strings.Contains(err.Error(), "fake-private-key") || strings.Contains(err.Error(), "provider-detail") {
		t.Fatal("quota error was not mapped safely")
	}
	upstream.Close()
	var redirected atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer redirectTarget.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client = NewOpenAIImageGenerator("fake-private-key", DefaultImageModel, redirect.Client(), redirect.URL)
	if _, err := client.Generate(context.Background(), "alice", "A photo"); err == nil || redirected.Load() != 0 {
		t.Fatal("provider redirect was followed")
	}
}

func TestImageModelAllowlistRejectsUnrecognizedModelsBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer upstream.Close()
	client := NewOpenAIImageGenerator("fake-server-key", DefaultImageModel, upstream.Client(), upstream.URL)
	for _, model := range []string{"dall-e-3", "gpt-image-made-up", "https://example.invalid/model"} {
		if _, err := client.GenerateWithOptions(context.Background(), "alice", "A photo", ImageGenerationOptions{Model: model}); err == nil {
			t.Fatal("unsupported model accepted")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("unsupported model reached the provider")
	}
}

func TestImageProviderFailuresAreSafeAndDoNotBecomeUserAuthFailures(t *testing.T) {
	for _, tc := range []struct{ status, want int }{{400, 422}, {401, 502}, {403, 502}, {429, 429}, {500, 502}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("private-provider-detail"))
			}))
			defer upstream.Close()
			client := NewOpenAIImageGenerator("server-key", "gpt-image-2.5-sunburst", upstream.Client(), upstream.URL)
			_, err := client.Generate(context.Background(), "alice", "A photo")
			var failure ImageError
			if !errors.As(err, &failure) || failure.Status != tc.want || strings.Contains(err.Error(), "private-provider-detail") {
				t.Fatal("unsafe provider failure", err)
			}
		})
	}
}

func TestImageResponseMustContainValidatedBytesNotAnExternalURL(t *testing.T) {
	for _, response := range []string{`{"data":[{"url":"http://internal.example/private"}]}`, `{"data":[{"b64_json":"invalid"}]}`, `{"data":[]}`, `not json`} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
		client := NewOpenAIImageGenerator("server-key", "gpt-image-2.5-sunburst", upstream.Client(), upstream.URL)
		if _, err := client.Generate(context.Background(), "alice", "A photo"); err == nil {
			t.Error("invalid response accepted")
		}
		upstream.Close()
	}
	disabled := NewOpenAIImageGenerator("", "gpt-image-2.5-sunburst", nil, "http://127.0.0.1:1")
	if disabled.Enabled() {
		t.Fatal("missing key enabled generation")
	}
	_, err := disabled.Generate(context.Background(), "alice", "A photo")
	var failure ImageError
	if !errors.As(err, &failure) || failure.Status != 503 {
		t.Fatal("missing key did not produce a configuration error")
	}
}
