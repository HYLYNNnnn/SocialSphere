package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"socialai/service"
)

type Config struct {
	Address               string
	StorageMode           string
	DataDir               string
	FrontendDir           string
	JWTSigningKey         string
	ElasticsearchURL      string
	ElasticsearchUser     string
	ElasticsearchPassword string
	UserIndex             string
	PostIndex             string
	GCSBucket             string
	OpenAIKey             string
	ImageModel            string
	AllowUserImageKeys    bool
	AllowedOrigins        []string
}

func value(name, fallback string) string {
	if result := strings.TrimSpace(os.Getenv(name)); result != "" {
		return result
	}
	return fallback
}

func Load() (Config, error) {
	port := value("PORT", "8080")
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return Config{}, errors.New("PORT must be between 1 and 65535")
	}
	allowUserImageKeys := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ALLOW_USER_IMAGE_KEYS"))) {
	case "", "false":
	case "true":
		allowUserImageKeys = true
	default:
		return Config{}, errors.New("ALLOW_USER_IMAGE_KEYS must be true or false")
	}
	c := Config{
		Address:     net.JoinHostPort(value("HOST", "127.0.0.1"), port),
		StorageMode: strings.ToLower(value("STORAGE_MODE", "local")),
		DataDir:     value("DATA_DIR", "./data"), FrontendDir: value("FRONTEND_DIR", "./public"),
		JWTSigningKey:         strings.TrimSpace(os.Getenv("JWT_SIGNING_KEY")),
		ElasticsearchURL:      strings.TrimSpace(os.Getenv("ES_URL")),
		ElasticsearchUser:     strings.TrimSpace(os.Getenv("ES_USERNAME")),
		ElasticsearchPassword: os.Getenv("ES_PASSWORD"),
		UserIndex:             value("ES_USER_INDEX", "socialsphere_users_v2"),
		PostIndex:             value("ES_POST_INDEX", "socialsphere_posts_v2"),
		GCSBucket:             strings.TrimSpace(os.Getenv("GCS_BUCKET")),
		OpenAIKey:             strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		ImageModel:            value("OPENAI_IMAGE_MODEL", "gpt-image-2.5-sunburst"),
		AllowUserImageKeys:    allowUserImageKeys,
	}
	if c.StorageMode != "local" && c.StorageMode != "cloud" {
		return Config{}, errors.New("STORAGE_MODE must be local or cloud")
	}
	if c.StorageMode == "cloud" {
		parsed, parseErr := url.Parse(c.ElasticsearchURL)
		if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			return Config{}, errors.New("cloud mode requires ES_URL with an http(s) address and separate credentials")
		}
		if c.GCSBucket == "" || c.JWTSigningKey == "" {
			return Config{}, errors.New("cloud mode requires GCS_BUCKET and a private JWT_SIGNING_KEY")
		}
	}
	if !service.IsSupportedImageModel(c.ImageModel) {
		return Config{}, errors.New("OPENAI_IMAGE_MODEL must be one of the supported GPT image models")
	}
	for _, origin := range strings.Split(value("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ",") {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		parsed, parseErr := url.Parse(origin)
		if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, errors.New("CORS_ORIGINS must contain explicit http(s) origins")
		}
		c.AllowedOrigins = append(c.AllowedOrigins, origin)
	}
	return c, nil
}
