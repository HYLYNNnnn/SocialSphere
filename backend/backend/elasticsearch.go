package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"socialai/model"

	"github.com/olivere/elastic/v7"
)

const elasticsearchTimeout = 15 * time.Second

const postMapping = `{
    "mappings": {"properties": {
        "id": {"type": "keyword"},
        "user": {"type": "keyword"},
        "message": {"type": "text"},
        "url": {"type": "keyword", "index": false},
        "type": {"type": "keyword", "index": false},
        "created_at": {"type": "date"}
    }}
}`

const userMapping = `{
    "mappings": {"properties": {
        "username": {"type": "keyword"},
        "password": {"type": "keyword", "index": false},
        "age": {"type": "long", "index": false},
        "gender": {"type": "keyword", "index": false}
    }}
}`

// ElasticsearchBackend stores user and post records in configurable indices.
// Existing indices are never deleted or silently recreated during startup.
type ElasticsearchBackend struct {
	client     *elastic.Client
	httpClient *http.Client
	userIndex  string
	postIndex  string
}

func NewElasticsearchBackend(ctx context.Context, endpoint, username, password, userIndex, postIndex string) (*ElasticsearchBackend, error) {
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(userIndex) == "" || strings.TrimSpace(postIndex) == "" {
		return nil, errors.New("Elasticsearch endpoint and index names are required")
	}
	if userIndex == postIndex {
		return nil, errors.New("Elasticsearch user and post indices must be different")
	}
	httpClient := &http.Client{Timeout: elasticsearchTimeout}
	options := []elastic.ClientOptionFunc{
		elastic.SetURL(endpoint),
		elastic.SetSniff(false),
		elastic.SetHealthcheck(false),
		elastic.SetHttpClient(httpClient),
	}
	if username != "" || password != "" {
		options = append(options, elastic.SetBasicAuth(username, password))
	}
	client, err := elastic.DialContext(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("initialize Elasticsearch client: %w", err)
	}
	result := &ElasticsearchBackend{client: client, httpClient: httpClient, userIndex: userIndex, postIndex: postIndex}
	if err := result.ensureIndex(ctx, userIndex, userMapping); err != nil {
		result.Close()
		return nil, err
	}
	if err := result.ensureIndex(ctx, postIndex, postMapping); err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

func (b *ElasticsearchBackend) ensureIndex(ctx context.Context, index, mapping string) error {
	ctx, cancel := context.WithTimeout(ctx, elasticsearchTimeout)
	defer cancel()
	exists, err := b.client.IndexExists(index).Do(ctx)
	if err != nil {
		return fmt.Errorf("check Elasticsearch index: %w", err)
	}
	if exists {
		return nil
	}
	_, err = b.client.CreateIndex(index).Body(mapping).Do(ctx)
	if err != nil {
		// Another application instance may have created the same index meanwhile.
		var responseError *elastic.Error
		if errors.As(err, &responseError) && responseError.Details != nil && responseError.Details.Type == "resource_already_exists_exception" {
			return nil
		}
		return fmt.Errorf("create Elasticsearch index: %w", err)
	}
	return nil
}

func (b *ElasticsearchBackend) Close() error {
	if b != nil {
		if b.client != nil {
			b.client.Stop()
		}
		if b.httpClient != nil {
			b.httpClient.CloseIdleConnections()
		}
	}
	return nil
}

func (b *ElasticsearchBackend) GetUser(ctx context.Context, username string) (*model.User, error) {
	ctx, cancel := context.WithTimeout(ctx, elasticsearchTimeout)
	defer cancel()
	result, err := b.client.Get().Index(b.userIndex).Id(username).Do(ctx)
	if elastic.IsNotFound(err) || (err == nil && !result.Found) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read user: %w", err)
	}
	var user model.User
	if err := json.Unmarshal(result.Source, &user); err != nil {
		return nil, fmt.Errorf("decode user record: %w", err)
	}
	return &user, nil
}

func (b *ElasticsearchBackend) CreateUser(ctx context.Context, user *model.User) (bool, error) {
	if user == nil || user.Username == "" {
		return false, errors.New("a username is required")
	}
	ctx, cancel := context.WithTimeout(ctx, elasticsearchTimeout)
	defer cancel()
	_, err := b.client.Index().Index(b.userIndex).Id(user.Username).
		OpType("create").BodyJson(user).Refresh("wait_for").Do(ctx)
	var responseError *elastic.Error
	if errors.As(err, &responseError) && responseError.Status == http.StatusConflict {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create user: %w", err)
	}
	return true, nil
}

func (b *ElasticsearchBackend) SavePost(ctx context.Context, post *model.Post) error {
	if post == nil || post.Id == "" {
		return errors.New("a post ID is required")
	}
	ctx, cancel := context.WithTimeout(ctx, elasticsearchTimeout)
	defer cancel()
	_, err := b.client.Index().Index(b.postIndex).Id(post.Id).BodyJson(post).Refresh("wait_for").Do(ctx)
	if err != nil {
		return fmt.Errorf("save post: %w", err)
	}
	return nil
}

func (b *ElasticsearchBackend) GetPost(ctx context.Context, id string) (*model.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, elasticsearchTimeout)
	defer cancel()
	result, err := b.client.Get().Index(b.postIndex).Id(id).Do(ctx)
	if elastic.IsNotFound(err) || (err == nil && !result.Found) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read post: %w", err)
	}
	var post model.Post
	if err := json.Unmarshal(result.Source, &post); err != nil {
		return nil, fmt.Errorf("decode post record: %w", err)
	}
	return &post, nil
}

func (b *ElasticsearchBackend) SearchPosts(ctx context.Context, user, keywords string, limit int) ([]model.Post, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := elastic.NewBoolQuery()
	if user != "" {
		query.Filter(elastic.NewTermQuery("user", user))
	}
	if strings.TrimSpace(keywords) != "" {
		query.Must(elastic.NewMatchQuery("message", keywords).Operator("AND"))
	}
	if user == "" && strings.TrimSpace(keywords) == "" {
		query.Must(elastic.NewMatchAllQuery())
	}
	ctx, cancel := context.WithTimeout(ctx, elasticsearchTimeout)
	defer cancel()
	result, err := b.client.Search().Index(b.postIndex).Query(query).Size(limit).
		SortBy(elastic.NewFieldSort("created_at").Desc().Missing("_last").UnmappedType("date"),
			elastic.NewFieldSort("id").Asc()).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search posts: %w", err)
	}
	posts := make([]model.Post, 0)
	if result.Hits == nil {
		return posts, nil
	}
	for _, hit := range result.Hits.Hits {
		var post model.Post
		if err := json.Unmarshal(hit.Source, &post); err != nil {
			return nil, fmt.Errorf("decode post search result: %w", err)
		}
		posts = append(posts, post)
	}
	return posts, nil
}

func (b *ElasticsearchBackend) DeletePost(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, elasticsearchTimeout)
	defer cancel()
	_, err := b.client.Delete().Index(b.postIndex).Id(id).Refresh("wait_for").Do(ctx)
	if elastic.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	return nil
}
