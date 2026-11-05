package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"socialai/backend"
	"socialai/config"
	"socialai/handler"
	"socialai/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	var users service.UserStore
	var posts service.PostStore
	var media service.MediaStore
	if cfg.StorageMode == "local" {
		store, err := backend.NewLocalStore(cfg.DataDir)
		if err != nil {
			log.Fatal("Unable to open local data: ", err)
		}
		users, posts, media = store, store, store
		if cfg.JWTSigningKey == "" {
			cfg.JWTSigningKey, err = backend.LoadOrCreateSigningKey(cfg.DataDir)
			if err != nil {
				log.Fatal("Unable to prepare local signing key: ", err)
			}
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		es, err := backend.NewElasticsearchBackend(ctx, cfg.ElasticsearchURL, cfg.ElasticsearchUser, cfg.ElasticsearchPassword, cfg.UserIndex, cfg.PostIndex)
		cancel()
		if err != nil {
			log.Fatal("Unable to initialize Elasticsearch: ", err)
		}
		defer es.Close()
		gcs, err := backend.NewGCSBackend(context.Background(), cfg.GCSBucket)
		if err != nil {
			log.Fatal("Unable to initialize cloud media: ", err)
		}
		defer gcs.Close()
		users, posts, media = es, es, gcs
	}
	auth, err := handler.NewAuthenticator(cfg.JWTSigningKey)
	if err != nil {
		log.Fatal(err)
	}
	router := handler.NewRouter(handler.RouterOptions{
		Users: handler.NewUserHandlers(service.NewUserService(users), auth), Auth: auth,
		Posts: service.NewPostService(posts, media), Media: media,
		Images:         service.NewOpenAIImageGenerator(cfg.OpenAIKey, cfg.ImageModel, nil, ""),
		AllowedOrigins: cfg.AllowedOrigins, FrontendDir: cfg.FrontendDir,
	})
	server := &http.Server{Addr: cfg.Address, Handler: router, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 180 * time.Second, IdleTimeout: 60 * time.Second}
	stopped := make(chan os.Signal, 1)
	shutdownDone := make(chan struct{})
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopped)
	go func() {
		defer close(shutdownDone)
		<-stopped
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Print("Graceful shutdown incomplete")
			_ = server.Close()
		}
	}()
	log.Printf("SocialSphere listening on %s (storage=%s)", cfg.Address, cfg.StorageMode)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-shutdownDone
}
