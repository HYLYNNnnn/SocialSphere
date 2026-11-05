package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"socialai/model"
)

type memoryUsers struct {
	mu      sync.Mutex
	users   map[string]model.User
	failure error
}

func newMemoryUsers() *memoryUsers { return &memoryUsers{users: map[string]model.User{}} }
func (m *memoryUsers) GetUser(ctx context.Context, username string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil {
		return nil, m.failure
	}
	user, exists := m.users[username]
	if !exists {
		return nil, nil
	}
	return &user, nil
}
func (m *memoryUsers) CreateUser(ctx context.Context, user *model.User) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil {
		return false, m.failure
	}
	if _, exists := m.users[user.Username]; exists {
		return false, nil
	}
	m.users[user.Username] = *user
	return true, nil
}

func TestRegisterHashesPasswordAndNormalizesUsername(t *testing.T) {
	store := newMemoryUsers()
	users := NewUserService(store)
	username, err := users.Register(context.Background(), model.User{Username: "  Alice_123 ", Password: "Example-pass-123", Age: 27})
	if err != nil || username != "alice_123" {
		t.Fatalf("register: %q %v", username, err)
	}
	saved := store.users[username]
	if saved.Password == "Example-pass-123" {
		t.Fatal("stored plaintext password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(saved.Password), []byte("Example-pass-123")); err != nil {
		t.Fatalf("invalid password hash: %v", err)
	}
	if _, err := users.Authenticate(context.Background(), "ALICE_123", "Example-pass-123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	for _, input := range []struct{ username, password string }{{"alice_123", "Wrong-pass-123"}, {"missing_user", "Example-pass-123"}} {
		if _, err := users.Authenticate(context.Background(), input.username, input.password); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong/unknown login error: %v", err)
		}
	}
}

func TestRegisterValidation(t *testing.T) {
	cases := []struct {
		name, username, password string
		age                      int64
	}{
		{"short username", "ab", "Example-pass-123", 0},
		{"invalid username", "alice!", "Example-pass-123", 0},
		{"leading underscore", "_alice", "Example-pass-123", 0},
		{"long username", strings.Repeat("a", 33), "Example-pass-123", 0},
		{"short password", "alice", "1234567", 0},
		{"blank password", "alice", "        ", 0},
		{"long password", "alice", strings.Repeat("a", 73), 0},
		{"multibyte password too long", "alice", strings.Repeat("密", 25), 0},
		{"invalid age", "alice", "Example-pass-123", 151},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewUserService(newMemoryUsers()).Register(context.Background(), model.User{Username: test.username, Password: test.password, Age: test.age})
			var validation ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("expected validation, got %v", err)
			}
		})
	}
}

func TestPasswordByteLimitDoesNotTruncateLogin(t *testing.T) {
	users := NewUserService(newMemoryUsers())
	password := strings.Repeat("a", 72)
	if _, err := users.Register(context.Background(), model.User{Username: "byte_limit", Password: password}); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Authenticate(context.Background(), "byte_limit", password); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Authenticate(context.Background(), "byte_limit", password+"suffix"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("long login accepted: %v", err)
	}
	chinese := strings.Repeat("密", 24)
	if _, err := users.Register(context.Background(), model.User{Username: "multi_byte", Password: chinese}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentRegistrationDoesNotOverwriteExistingAccount(t *testing.T) {
	store := newMemoryUsers()
	users := NewUserService(store)
	passwords := []string{"First-pass-123", "Second-pass-123"}
	outcomes := make(chan struct {
		password string
		err      error
	}, 2)
	var wait sync.WaitGroup
	for _, password := range passwords {
		wait.Add(1)
		go func(password string) {
			defer wait.Done()
			_, err := users.Register(context.Background(), model.User{Username: "same_user", Password: password})
			outcomes <- struct {
				password string
				err      error
			}{password, err}
		}(password)
	}
	wait.Wait()
	close(outcomes)
	winner := ""
	duplicates := 0
	for result := range outcomes {
		if result.err == nil {
			winner = result.password
		} else if errors.Is(result.err, ErrUsernameTaken) {
			duplicates++
		} else {
			t.Fatal(result.err)
		}
	}
	if winner == "" || duplicates != 1 || len(store.users) != 1 {
		t.Fatalf("unexpected registration results: winner=%q duplicate=%d", winner, duplicates)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(store.users["same_user"].Password), []byte(winner)); err != nil {
		t.Fatal("duplicate request overwrote winner")
	}
}

func TestStorageFailuresDoNotBecomeSuccessfulLoginOrRegistration(t *testing.T) {
	store := newMemoryUsers()
	store.failure = errors.New("storage unavailable")
	users := NewUserService(store)
	if _, err := users.Authenticate(context.Background(), "alice_123", "Example-pass-123"); !errors.Is(err, store.failure) {
		t.Fatalf("login error: %v", err)
	}
	if _, err := users.Register(context.Background(), model.User{Username: "alice_123", Password: "Example-pass-123"}); !errors.Is(err, store.failure) {
		t.Fatalf("registration error: %v", err)
	}
}
