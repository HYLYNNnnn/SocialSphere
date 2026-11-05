package config

import "testing"

func clearConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "HOST", "STORAGE_MODE", "DATA_DIR", "FRONTEND_DIR", "JWT_SIGNING_KEY", "ES_URL", "ES_USERNAME", "ES_PASSWORD", "ES_USER_INDEX", "ES_POST_INDEX", "GCS_BUCKET", "OPENAI_API_KEY", "OPENAI_IMAGE_MODEL", "CORS_ORIGINS"} {
		t.Setenv(key, "")
	}
}
func TestDefaultConfigurationNeedsNoCloudCredentials(t *testing.T) {
	clearConfig(t)
	c, err := Load()
	if err != nil || c.StorageMode != "local" || c.OpenAIKey != "" || c.ImageModel != "gpt-image-2.5-sunburst" {
		t.Fatal("unexpected default", err)
	}
}
func TestConfigurationRejectsInvalidCloudAndOrigins(t *testing.T) {
	for _, tc := range []struct{ name, value string }{{"PORT", "bad"}, {"STORAGE_MODE", "unknown"}, {"STORAGE_MODE", "cloud"}, {"CORS_ORIGINS", "*"}, {"OPENAI_IMAGE_MODEL", "dall-e-3"}} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			clearConfig(t)
			t.Setenv(tc.name, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
