package backend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"socialai/model"
)

func testElasticsearch(t *testing.T, serve http.HandlerFunc) *ElasticsearchBackend {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		serve(w, r)
	}))
	t.Cleanup(server.Close)
	backend, err := NewElasticsearchBackend(context.Background(), server.URL, "", "", "users", "posts")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}

func TestElasticsearchCreatesSafeMappingsWithoutDiscoveryRequests(t *testing.T) {
	mappings := make(chan map[string]interface{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		username, password, ok := r.BasicAuth()
		if !ok || username != "test-user" || password != "test-password" {
			t.Error("expected configured basic authentication")
		}
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			var mapping map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
				t.Error(err)
			}
			mappings <- mapping
			_, _ = w.Write([]byte(`{"acknowledged":true,"shards_acknowledged":true}`))
		default:
			t.Errorf("unexpected discovery request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	backend, err := NewElasticsearchBackend(context.Background(), server.URL, "test-user", "test-password", "users", "posts")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	userProperties := (<-mappings)["mappings"].(map[string]interface{})["properties"].(map[string]interface{})
	if userProperties["password"].(map[string]interface{})["index"] != false {
		t.Error("password hashes must not be indexed")
	}
	postProperties := (<-mappings)["mappings"].(map[string]interface{})["properties"].(map[string]interface{})
	if postProperties["created_at"].(map[string]interface{})["type"] != "date" {
		t.Error("expected a timestamp mapping")
	}
	if postProperties["id"].(map[string]interface{})["type"] != "keyword" {
		t.Error("expected sortable post IDs")
	}
}

func TestElasticsearchUserCreationIsAtomicAndWaitsForVisibility(t *testing.T) {
	var attempts atomic.Int32
	backend := testElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/users/_doc/alice" {
			t.Errorf("unexpected create request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("op_type") != "create" || r.URL.Query().Get("refresh") != "wait_for" {
			t.Error("user registration must create uniquely and wait for visibility")
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"_index":"users","_id":"alice","result":"created"}`))
		} else {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"type":"version_conflict_engine_exception","reason":"exists"},"status":409}`))
		}
	})
	user := &model.User{Username: "alice", Password: "bcrypt-test-hash"}
	created, err := backend.CreateUser(context.Background(), user)
	if err != nil || !created {
		t.Fatalf("first registration: created=%t, err=%v", created, err)
	}
	created, err = backend.CreateUser(context.Background(), user)
	if err != nil || created {
		t.Fatalf("duplicate registration: created=%t, err=%v", created, err)
	}
}

func TestElasticsearchReadsRecordsAndTreatsMissingAsAbsent(t *testing.T) {
	backend := testElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/_doc/alice":
			_, _ = w.Write([]byte(`{"found":true,"_source":{"username":"alice","password":"bcrypt-test-hash","age":20}}`))
		case "/posts/_doc/post-1":
			_, _ = w.Write([]byte(`{"found":true,"_source":{"id":"post-1","user":"alice","message":"Hello","url":"/media/post-1","type":"image","created_at":"2026-11-04T12:00:00Z"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"found":false}`))
		}
	})
	user, err := backend.GetUser(context.Background(), "alice")
	if err != nil || user == nil || user.Password != "bcrypt-test-hash" {
		t.Fatalf("read user: %+v, %v", user, err)
	}
	missingUser, err := backend.GetUser(context.Background(), "missing")
	if err != nil || missingUser != nil {
		t.Fatalf("missing user: %+v, %v", missingUser, err)
	}
	post, err := backend.GetPost(context.Background(), "post-1")
	if err != nil || post == nil || post.Id != "post-1" {
		t.Fatalf("read post: %+v, %v", post, err)
	}
	missingPost, err := backend.GetPost(context.Background(), "missing")
	if err != nil || missingPost != nil {
		t.Fatalf("missing post: %+v, %v", missingPost, err)
	}
}

func TestElasticsearchSearchUsesBoundedCombinedQueryAndStableRecentSort(t *testing.T) {
	observed := make(chan map[string]interface{}, 1)
	backend := testElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/posts/_search" {
			t.Errorf("unexpected search URL %s", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		observed <- body
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_source":{"id":"post-1","user":"alice","message":"Hello world","type":"image"}}]}}`))
	})
	posts, err := backend.SearchPosts(context.Background(), "alice", "Hello world", 1000)
	if err != nil || len(posts) != 1 {
		t.Fatalf("search: %+v, %v", posts, err)
	}
	body := <-observed
	if body["size"] != float64(100) {
		t.Errorf("unbounded query size: %v", body["size"])
	}
	query := body["query"].(map[string]interface{})["bool"].(map[string]interface{})
	encoded, _ := json.Marshal(query)
	if !strings.Contains(string(encoded), `"user":"alice"`) || !strings.Contains(string(encoded), `"operator":"AND"`) {
		t.Errorf("expected combined user and keywords query: %s", encoded)
	}
	sorts := body["sort"].([]interface{})
	timestampSort := sorts[0].(map[string]interface{})["created_at"].(map[string]interface{})
	if timestampSort["order"] != "desc" || timestampSort["missing"] != "_last" {
		t.Errorf("unexpected timestamp order: %v", timestampSort)
	}
	if sorts[1].(map[string]interface{})["id"].(map[string]interface{})["order"] != "asc" {
		t.Error("equal timestamps must have a stable ID tie breaker")
	}
}

func TestElasticsearchEmptySearchReturnsAnArray(t *testing.T) {
	backend := testElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":0,"relation":"eq"},"hits":[]}}`))
	})
	posts, err := backend.SearchPosts(context.Background(), "", "", 0)
	if err != nil || posts == nil || len(posts) != 0 {
		t.Fatalf("expected empty array: %+v, %v", posts, err)
	}
}

func TestElasticsearchPostWritesWaitForVisibilityAndDeleteIsIdempotent(t *testing.T) {
	observed := make(chan string, 2)
	backend := testElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/posts/_doc/post-1" || r.URL.Query().Get("refresh") != "wait_for" {
			t.Errorf("unexpected post mutation: %s %s", r.Method, r.URL.String())
		}
		observed <- r.Method
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"_index":"posts","_id":"post-1","result":"not_found","status":404}`))
		} else {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"_index":"posts","_id":"post-1","result":"created"}`))
		}
	})
	if err := backend.SavePost(context.Background(), &model.Post{Id: "post-1", User: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.DeletePost(context.Background(), "post-1"); err != nil {
		t.Fatal(err)
	}
	if <-observed != http.MethodPut || <-observed != http.MethodDelete {
		t.Error("missing post writes")
	}
}

func TestElasticsearchHonorsCancelledContext(t *testing.T) {
	backend := testElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("cancelled search must not reach the server")
		_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := backend.SearchPosts(ctx, "", "", 10)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
