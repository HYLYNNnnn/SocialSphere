package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"socialai/backend"
	"socialai/service"
)

func byokRouter(t *testing.T, generator service.ImageGenerator, allowPersonal bool) (http.Handler, string) {
	t.Helper()
	store, err := backend.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	auth := testAuthenticator(t)
	token, err := auth.Issue("alice")
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(RouterOptions{
		Users: NewUserHandlers(service.NewUserService(store), auth), Auth: auth,
		Posts: service.NewPostService(store, store), Media: store, Images: generator,
		AllowedOrigins: []string{"http://localhost:3000"}, AllowUserImageKeys: allowPersonal,
		DefaultImageModel: service.DefaultImageModel,
	}), token
}

func imageRequest(router http.Handler, token, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/generate-image", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		r.Header.Set("X-OpenAI-Api-Key", key)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestImageConfigIsAuthenticatedAndExposesOnlyPublicSettings(t *testing.T) {
	for _, configured := range []bool{false, true} {
		key := ""
		if configured {
			key = "fake-private-server-key"
		}
		generator := service.NewOpenAIImageGenerator(key, service.DefaultImageModel, nil, "http://127.0.0.1:1")
		router, token := byokRouter(t, generator, true)
		for _, path := range []string{"/api/image-config", "/image-config"} {
			if got := socialCall(router, http.MethodGet, path, "", "", nil); got.Code != http.StatusUnauthorized {
				t.Fatal("anonymous image configuration request accepted")
			}
			got := socialCall(router, http.MethodGet, path, token, "", nil)
			var result struct {
				ServerConfigured bool                 `json:"server_configured"`
				UserKeysAllowed  bool                 `json:"user_keys_allowed"`
				DefaultModel     string               `json:"default_model"`
				Models           []service.ImageModel `json:"models"`
			}
			if err := json.Unmarshal(got.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(got.Body.Bytes(), &fields)
			if got.Code != http.StatusOK || result.ServerConfigured != configured || !result.UserKeysAllowed ||
				result.DefaultModel != service.DefaultImageModel || len(result.Models) != 5 || len(fields) != 4 {
				t.Fatal("unexpected image configuration schema")
			}
			for _, model := range result.Models {
				if !service.IsSupportedImageModel(model.ID) || model.Label == "" {
					t.Fatal("invalid model metadata")
				}
			}
			if got.Header().Get("Cache-Control") != "no-store" || strings.Contains(got.Body.String(), "fake-private-server-key") {
				t.Fatal("image config was cacheable or exposed a secret")
			}
		}
	}
}

func TestImageKeySourceAndModelValidationNeverFallBackToServer(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer upstream.Close()
	generator := service.NewOpenAIImageGenerator("fake-server-key", service.DefaultImageModel, upstream.Client(), upstream.URL)
	for _, tc := range []struct {
		name, body, key string
		allow           bool
		want            int
	}{
		{"disabled personal", `{"prompt":"A photo","key_source":"personal"}`, "fake-personal-key", false, 403},
		{"missing personal key", `{"prompt":"A photo","key_source":"personal"}`, "", true, 400},
		{"blank personal key", `{"prompt":"A photo","key_source":"personal"}`, " ", true, 400},
		{"malformed personal key", `{"prompt":"A photo","key_source":"personal"}`, "fake personal key", true, 400},
		{"server rejects personal key", `{"prompt":"A photo","key_source":"server"}`, "fake-personal-key", true, 400},
		{"legacy server rejects personal key", `{"prompt":"A photo"}`, "fake-personal-key", true, 400},
		{"unknown source", `{"prompt":"A photo","key_source":"other"}`, "", true, 400},
		{"unknown model", `{"prompt":"A photo","model":"gpt-image-made-up"}`, "", true, 400},
		{"retired model", `{"prompt":"A photo","model":"dall-e-3"}`, "", true, 400},
		{"key in JSON", `{"prompt":"A photo","key_source":"personal","api_key":"fake-private-body-key"}`, "", true, 400},
		{"endpoint in JSON", `{"prompt":"A photo","endpoint":"https://example.invalid"}`, "", true, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, token := byokRouter(t, generator, tc.allow)
			got := imageRequest(router, token, tc.body, tc.key)
			if got.Code != tc.want || got.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status %d, want %d and no-store", got.Code, tc.want)
			}
			for _, secret := range []string{"fake-server-key", "fake-personal-key", "fake-private-body-key"} {
				if strings.Contains(got.Body.String(), secret) {
					t.Fatal("request key was reflected")
				}
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("rejected request used the upstream or server key")
	}
}

func TestPersonalImageKeyWorksWithoutServerKeyAndKeepsRequestsIsolated(t *testing.T) {
	image := socialPNG(t)
	var received sync.Map
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received.Store(body["prompt"], r.Header.Get("Authorization")+"|"+body["model"].(string))
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(image)}}})
	}))
	defer upstream.Close()
	generator := service.NewOpenAIImageGenerator("", service.DefaultImageModel, upstream.Client(), upstream.URL)
	router, token := byokRouter(t, generator, true)
	var calls sync.WaitGroup
	for _, tc := range []struct{ prompt, key, model string }{
		{"first", "fake-first-personal-key", "gpt-image-2"},
		{"second", "fake-second-personal-key", "gpt-image-2.6-sunburst"},
	} {
		calls.Add(1)
		go func() {
			defer calls.Done()
			body, _ := json.Marshal(map[string]string{"prompt": tc.prompt, "model": tc.model, "key_source": "personal"})
			got := imageRequest(router, token, string(body), tc.key)
			if got.Code != 200 || got.Header().Get("Cache-Control") != "no-store" || strings.Contains(got.Body.String(), tc.key) {
				t.Errorf("personal generation returned status %d or leaked/cached the key", got.Code)
			}
		}()
	}
	calls.Wait()
	for prompt, want := range map[string]string{
		"first":  "Bearer fake-first-personal-key|gpt-image-2",
		"second": "Bearer fake-second-personal-key|gpt-image-2.6-sunburst",
	} {
		if got, ok := received.Load(prompt); !ok || got != want {
			t.Fatal("personal requests mixed keys or models")
		}
	}
	if generator.Enabled() {
		t.Fatal("personal request changed the server configuration")
	}
	got := imageRequest(router, token, `{"prompt":"server request"}`, "")
	if got.Code != 503 {
		t.Fatal("personal key persisted as server configuration")
	}
}

func TestProviderFailuresDoNotLogOutTheWebsiteOrLeakUpstreamText(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"insufficient_quota","message":"fake-personal-secret provider-private-detail"}}`))
		}))
		generator := service.NewOpenAIImageGenerator("", service.DefaultImageModel, upstream.Client(), upstream.URL)
		router, token := byokRouter(t, generator, true)
		got := imageRequest(router, token, `{"prompt":"A photo","key_source":"personal"}`, "fake-personal-secret")
		if got.Code == 401 || got.Code == 200 || strings.Contains(got.Body.String(), "fake-personal-secret") || strings.Contains(got.Body.String(), "provider-private-detail") {
			t.Fatal("provider failure was exposed or became website authentication failure")
		}
		if me := socialCall(router, http.MethodGet, "/api/me", token, "", nil); me.Code != 200 {
			t.Fatal("provider failure affected website JWT")
		}
		upstream.Close()
	}
}

func TestImagePersonalKeyHeaderAllowedByCORS(t *testing.T) {
	router, _ := byokRouter(t, nil, true)
	req := httptest.NewRequest(http.MethodOptions, "/api/generate-image", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-openai-api-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(strings.ToLower(w.Header().Get("Access-Control-Allow-Headers")), "x-openai-api-key") {
		t.Fatal("CORS rejected the personal key header")
	}
}
