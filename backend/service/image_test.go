package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
