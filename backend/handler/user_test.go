package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"socialai/model"
	"socialai/service"
)

type handlerUserStore struct{ users map[string]model.User }

func (s *handlerUserStore) GetUser(ctx context.Context, name string) (*model.User, error) {
	user, ok := s.users[name]
	if !ok {
		return nil, nil
	}
	return &user, nil
}
func (s *handlerUserStore) CreateUser(ctx context.Context, user *model.User) (bool, error) {
	if _, ok := s.users[user.Username]; ok {
		return false, nil
	}
	s.users[user.Username] = *user
	return true, nil
}

func newTestUserHandlers(t *testing.T) (*UserHandlers, *Authenticator, *handlerUserStore) {
	store := &handlerUserStore{users: map[string]model.User{}}
	auth := testAuthenticator(t)
	return NewUserHandlers(service.NewUserService(store), auth), auth, store
}
func callUser(handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func TestUserHandlersRegisterLoginAndProfileExcludePassword(t *testing.T) {
	handlers, auth, store := newTestUserHandlers(t)
	signup := callUser(handlers.Signup, `{"username":"Alice_123","password":"Example-pass-123"}`)
	if signup.Code != http.StatusCreated {
		t.Fatalf("signup=%d %s", signup.Code, signup.Body.String())
	}
	if strings.Contains(signup.Body.String(), "password") || strings.Contains(signup.Body.String(), "Example-pass-123") {
		t.Fatal("signup exposed password")
	}
	if store.users["alice_123"].Password == "Example-pass-123" {
		t.Fatal("plaintext stored")
	}
	duplicate := callUser(handlers.Signup, `{"username":"ALICE_123","password":"Other-pass-123"}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate=%d", duplicate.Code)
	}
	signin := callUser(handlers.Signin, `{"username":" alice_123 ","password":"Example-pass-123"}`)
	if signin.Code != http.StatusOK {
		t.Fatalf("signin=%d %s", signin.Code, signin.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(signin.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["token"] == "" || body["username"] != "alice_123" {
		t.Fatalf("unexpected signin response: %v", body)
	}
	if strings.Contains(signin.Body.String(), "password") {
		t.Fatal("signin exposed password")
	}
	request := httptest.NewRequest(http.MethodGet, "/me", nil)
	request.Header.Set("Authorization", "Bearer "+body["token"])
	profile := httptest.NewRecorder()
	auth.Middleware(http.HandlerFunc(handlers.Me)).ServeHTTP(profile, request)
	if profile.Code != 200 || strings.Contains(profile.Body.String(), "password") {
		t.Fatalf("profile=%d %s", profile.Code, profile.Body.String())
	}
	bad := callUser(handlers.Signin, `{"username":"alice_123","password":"Wrong-pass-123"}`)
	unknown := callUser(handlers.Signin, `{"username":"missing_user","password":"Wrong-pass-123"}`)
	if bad.Code != 401 || unknown.Code != 401 || bad.Body.String() != unknown.Body.String() {
		t.Fatal("wrong and unknown credentials must share a generic 401")
	}
}

func TestUserHandlersRejectMalformedJSONWithoutPanicking(t *testing.T) {
	handlers, _, _ := newTestUserHandlers(t)
	for _, body := range []string{`{`, `null`, `[]`, `{"username":"alice_123","password":"Example-pass-123"} {}`, `{"username":"alice_123","password":"Example-pass-123","admin":true}`} {
		response := callUser(handlers.Signup, body)
		if response.Code != 400 {
			t.Fatalf("body=%q status=%d", body, response.Code)
		}
	}
	huge := callUser(handlers.Signup, `{"username":"`+strings.Repeat("a", 20<<10)+`","password":"Example-pass-123"}`)
	if huge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status=%d", huge.Code)
	}
}
