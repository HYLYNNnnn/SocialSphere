package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"socialai/backend"
	"socialai/model"
	"socialai/service"
)

type socialImageStub struct{}

func (socialImageStub) Enabled() bool { return true }
func (socialImageStub) Generate(context.Context, string, string) (string, error) {
	return "data:image/png;base64,dGVzdA==", nil
}

func socialFixture(t *testing.T, image service.ImageGenerator) (http.Handler, *Authenticator, *backend.LocalStore) {
	t.Helper()
	store, err := backend.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthenticator(strings.Repeat("x", 48))
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(RouterOptions{Users: NewUserHandlers(service.NewUserService(store), auth), Auth: auth, Posts: service.NewPostService(store, store), Media: store, Images: image, AllowedOrigins: []string{"http://localhost:3000"}})
	return router, auth, store
}
func socialPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func socialCall(router http.Handler, method, path, token, contentType string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	return result
}
func socialMultipart(t *testing.T, message, filename string, media []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("message", message); err != nil {
		t.Fatal(err)
	}
	if filename != "" {
		part, err := writer.CreateFormFile("media_file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(media); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()
	return &body, writer.FormDataContentType()
}

func TestSocialHTTPRegistrationLoginAndAccountResponse(t *testing.T) {
	router, _, store := socialFixture(t, nil)
	result := socialCall(router, "POST", "/api/signup", "", "application/json", strings.NewReader(`{"username":"Alice","password":"Example-pass-123","age":20,"gender":""}`))
	if result.Code != 201 || strings.Contains(result.Body.String(), "password") {
		t.Fatalf("signup: %d %s", result.Code, result.Body.String())
	}
	user, _ := store.GetUser(context.Background(), "alice")
	if user == nil || user.Password == "Example-pass-123" {
		t.Fatal("password was not hashed")
	}
	result = socialCall(router, "POST", "/api/signin", "", "application/json", strings.NewReader(`{"username":"alice","password":"Example-pass-123"}`))
	var signin map[string]string
	json.Unmarshal(result.Body.Bytes(), &signin)
	if result.Code != 200 || signin["token"] == "" || signin["username"] != "alice" {
		t.Fatal("signin failed")
	}
	me := socialCall(router, "GET", "/api/me", signin["token"], "", nil)
	if me.Code != 200 || strings.Contains(me.Body.String(), "password") {
		t.Fatal("unsafe account response")
	}
	if result := socialCall(router, "GET", "/api/search", "", "", nil); result.Code != 401 {
		t.Fatal("anonymous feed accepted")
	}
}

func TestSocialUploadSearchDeleteAndMediaRange(t *testing.T) {
	router, auth, _ := socialFixture(t, nil)
	alice, _ := auth.Issue("alice")
	bob, _ := auth.Issue("bob")
	image := socialPNG(t)
	body, contentType := socialMultipart(t, "Beach and travel", "PHOTO.JPG", image)
	result := socialCall(router, "POST", "/api/upload", alice, contentType, body)
	var post model.Post
	if err := json.Unmarshal(result.Body.Bytes(), &post); err != nil {
		t.Fatal(err, result.Body.String())
	}
	if result.Code != 201 || post.User != "alice" || post.Type != "image" || post.CreatedAt.IsZero() {
		t.Fatalf("upload: %d", result.Code)
	}
	result = socialCall(router, "GET", "/api/search?user=alice&keywords=travel%20beach", bob, "", nil)
	var posts []model.Post
	json.Unmarshal(result.Body.Bytes(), &posts)
	if result.Code != 200 || len(posts) != 1 {
		t.Fatal("new post not immediately searchable")
	}
	req := httptest.NewRequest("GET", post.Url, nil)
	req.Header.Set("Range", "bytes=0-3")
	media := httptest.NewRecorder()
	router.ServeHTTP(media, req)
	if media.Code != 206 || !bytes.Equal(media.Body.Bytes(), image[:4]) || media.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("media range: %d", media.Code)
	}
	if result := socialCall(router, "DELETE", "/api/post/"+post.Id, bob, "", nil); result.Code != 404 {
		t.Fatal("other user deleted post")
	}
	if result := socialCall(router, "DELETE", "/api/post/"+post.Id, alice, "", nil); result.Code != 204 {
		t.Fatalf("owner deletion failed %d", result.Code)
	}
	if result := socialCall(router, "GET", post.Url, "", "", nil); result.Code != 404 {
		t.Fatal("deleted media still exists")
	}
	result = socialCall(router, "GET", "/api/search", alice, "", nil)
	if strings.TrimSpace(result.Body.String()) != "[]" {
		t.Fatal("empty search did not return an array")
	}
}

func TestSocialUploadValidationRejectsInvalidAndOversizedFiles(t *testing.T) {
	router, auth, _ := socialFixture(t, nil)
	token, _ := auth.Issue("alice")
	for _, tc := range []struct {
		name string
		data []byte
		want int
	}{{"fake.jpg", []byte("<html>not an image</html>"), 400}, {"big.png", bytes.Repeat([]byte{0}, int(service.MaxMediaBytes)+1), 413}, {"", nil, 400}} {
		body, contentType := socialMultipart(t, "A post", tc.name, tc.data)
		if result := socialCall(router, "POST", "/api/upload", token, contentType, body); result.Code != tc.want {
			t.Fatalf("%s: got %d want %d", tc.name, result.Code, tc.want)
		}
	}
	body, contentType := socialMultipart(t, "", "empty-message.png", socialPNG(t))
	if result := socialCall(router, "POST", "/api/upload", token, contentType, body); result.Code != 400 {
		t.Fatal("empty message accepted")
	}
}

func TestSocialFeedIsRecentAndNotLimitedToTenPosts(t *testing.T) {
	router, auth, store := socialFixture(t, nil)
	token, _ := auth.Issue("alice")
	posts := service.NewPostService(store, store)
	for i := 0; i < 12; i++ {
		if _, err := posts.Create(context.Background(), "alice", "Photo "+strconv.Itoa(i), socialPNG(t)); err != nil {
			t.Fatal(err)
		}
	}
	result := socialCall(router, "GET", "/api/search", token, "", nil)
	var feed []model.Post
	json.Unmarshal(result.Body.Bytes(), &feed)
	if result.Code != 200 || len(feed) != 12 {
		t.Fatalf("feed count %d", len(feed))
	}
	for i := 1; i < len(feed); i++ {
		if feed[i].CreatedAt.After(feed[i-1].CreatedAt) {
			t.Fatal("feed is not recent first")
		}
	}
}

func TestSocialImageDisabledAndRateLimit(t *testing.T) {
	router, auth, _ := socialFixture(t, nil)
	token, _ := auth.Issue("alice")
	if r := socialCall(router, "POST", "/api/generate-image", token, "application/json", strings.NewReader(`{"prompt":"A photo"}`)); r.Code != 503 {
		t.Fatal("unconfigured AI was not explicit")
	}
	router, auth, _ = socialFixture(t, socialImageStub{})
	token, _ = auth.Issue("alice")
	for i := 0; i < 3; i++ {
		r := socialCall(router, "POST", "/api/generate-image", token, "application/json", strings.NewReader(`{"prompt":"A photo"}`))
		want := 200
		if i == 2 {
			want = 429
		}
		if r.Code != want {
			t.Fatalf("request %d status %d", i, r.Code)
		}
	}
}
