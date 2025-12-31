package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heathcliff26/promremote/promremote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type config struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	var cfg config
	b, err := os.ReadFile("test-config.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, &cfg)
	if err != nil {
		panic(err)
	}

	buildInfoCollector := collectors.NewBuildInfoCollector()
	reg := prometheus.NewRegistry()
	reg.MustRegister(buildInfoCollector)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		err = server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Failed to start http server", "err", err)
			os.Exit(1)
		}
	}()
	defer func() {
		err = server.Shutdown(context.Background())
		if err != nil {
			slog.Error("Failed to shutdown http server", "err", err)
		}
	}()

	var opts []promremote.ClientOption
	if cfg.Username != "" {
		opts = append(opts, promremote.WithBasicAuth(cfg.Username, cfg.Password))
	}

	rw, err := promremote.NewWriteClient(cfg.URL, "promremote-test", "promremote-test", reg, opts...)
	if err != nil {
		panic(err)
	}

	rwQuit := make(chan bool)
	err = rw.Run(30*time.Second, rwQuit)
	if err != nil {
		panic(err)
	}
	defer func() {
		close(rwQuit)
		slog.Info("Executed shutdown")
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit

	slog.Info("Received stop signal, shutting down")
}
