package config

import "testing"

func clearConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "HOST", "STORAGE_MODE", "DATA_DIR", "FRONTEND_DIR", "JWT_SIGNING_KEY", "ES_URL", "ES_USERNAME", "ES_PASSWORD", "ES_USER_INDEX", "ES_POST_INDEX", "GCS_BUCKET", "OPENAI_API_KEY", "OPENAI_IMAGE_MODEL", "ALLOW_USER_IMAGE_KEYS", "CORS_ORIGINS"} {
		t.Setenv(key, "")
	}
}
func TestDefaultConfigurationNeedsNoCloudCredentials(t *testing.T) {
	clearConfig(t)
	c, err := Load()
	if err != nil || c.StorageMode != "local" || c.OpenAIKey != "" || c.ImageModel != "gpt-image-2.5-sunburst" || c.AllowUserImageKeys {
		t.Fatal("unexpected default", err)
	}
}
func TestConfigurationRejectsInvalidCloudAndOrigins(t *testing.T) {
	for _, tc := range []struct{ name, value string }{{"PORT", "bad"}, {"STORAGE_MODE", "unknown"}, {"STORAGE_MODE", "cloud"}, {"CORS_ORIGINS", "*"}, {"OPENAI_IMAGE_MODEL", "dall-e-3"}, {"OPENAI_IMAGE_MODEL", "gpt-image-unlisted"}} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			clearConfig(t)
			t.Setenv(tc.name, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestUserImageKeysRequireExplicitBooleanConfiguration(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"false", false},
		{"true", true},
		{" TRUE ", true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			clearConfig(t)
			t.Setenv("ALLOW_USER_IMAGE_KEYS", tc.value)
			cfg, err := Load()
			if err != nil || cfg.AllowUserImageKeys != tc.want {
				t.Fatalf("unexpected personal-key policy: enabled=%v error=%v", cfg.AllowUserImageKeys, err)
			}
		})
	}
	for _, value := range []string{"yes", "1", "enabled", "truthy"} {
		t.Run("invalid_"+value, func(t *testing.T) {
			clearConfig(t)
			t.Setenv("ALLOW_USER_IMAGE_KEYS", value)
			if _, err := Load(); err == nil || err.Error() != "ALLOW_USER_IMAGE_KEYS must be true or false" {
				t.Fatalf("expected a clear boolean configuration error, got %v", err)
			}
		})
	}
}

func TestConfigurationAcceptsOnlySupportedImageModels(t *testing.T) {
	for _, model := range []string{"gpt-image-2", "gpt-image-2.5-sunburst", "gpt-image-2.5-flare", "gpt-image-2.6-sunburst", "gpt-image-2.6-flare"} {
		t.Run(model, func(t *testing.T) {
			clearConfig(t)
			t.Setenv("OPENAI_IMAGE_MODEL", model)
			cfg, err := Load()
			if err != nil || cfg.ImageModel != model {
				t.Fatalf("supported model rejected: %v", err)
			}
		})
	}
}
