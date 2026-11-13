package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"umc-pmday/internal/db"
	"umc-pmday/internal/handlers"
	"umc-pmday/internal/queue"
	"umc-pmday/web"
)

func loadAdminPIN() string {
	for _, path := range []string{os.Getenv("ADMIN_PIN_FILE"), "secrets/admin_pin"} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if b, err := os.ReadFile(path); err == nil {
			if v := strings.TrimSpace(string(b)); v != "" {
				return v
			}
		}
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_PIN")); v != "" {
		return v
	}
	return "changeme"
}

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/umc_pmday?parseTime=true&loc=Local&charset=utf8mb4"
	}
	adminPIN := loadAdminPIN()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	store, err := db.Open(dsn, adminPIN)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer store.DB.Close()

	hub := queue.NewHub(store)
	coord, err := queue.NewCoordinator(store, hub)
	if err != nil {
		log.Fatalf("coordinator init failed: %v", err)
	}

	srvHandler, err := handlers.NewServer(store, coord, hub, web.FS)
	if err != nil {
		log.Fatalf("server handler init failed: %v", err)
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           srvHandler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0, // Required for long-lived SSE streams
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("UMC PM Day server listening on http://localhost:%s", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	log.Println("server stopped cleanly")
}
