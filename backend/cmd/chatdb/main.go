package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chatdb/internal/api"
	"chatdb/internal/auth"
	"chatdb/internal/config"
	"chatdb/internal/engine"
	"chatdb/internal/migrate"
	"chatdb/internal/security"
	"chatdb/internal/store"
	"chatdb/web"
)

func main() {
	configPath := flag.String("config", "chatdb.config.json", "Path to chatdb config JSON")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := store.OpenMetadataDB(cfg.Metadata.Path)
	if err != nil {
		log.Fatalf("open metadata db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := migrate.Bootstrap(ctx, db); err != nil {
		cancel()
		log.Fatalf("bootstrap metadata: %v", err)
	}
	if err := migrate.Upgrade(ctx, db); err != nil {
		cancel()
		log.Fatalf("upgrade metadata: %v", err)
	}
	cancel()
	log.Printf("metadata sqlite ready: %s", cfg.Metadata.Path)

	crypter, err := security.NewCrypter([]byte(cfg.AppKey))
	if err != nil {
		log.Fatalf("crypter: %v", err)
	}

	st := store.New(db)

	srv := &api.Server{
		Cfg:     cfg,
		Store:   st,
		Crypter: crypter,
		JWT:     auth.NewIssuer(cfg.JWTSecret),
		Pools:   engine.NewManager(),
		Static:  web.SPA(),
	}

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("chatdb listening on http://%s", cfg.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	srv.Pools.CloseAll()
}
