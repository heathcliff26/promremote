package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
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

	rw, err := promremote.NewWriteClient(cfg.URL, "promremote-test", "promremote-test", reg)
	if err != nil {
		panic(err)
	}
	if cfg.Username != "" {
		err = rw.SetBasicAuth(cfg.Username, cfg.Password)
		if err != nil {
			panic(err)
		}
	}

	rwQuit := make(chan bool)
	rw.Run(30*time.Second, rwQuit)
	defer func() {
		rwQuit <- true
		close(rwQuit)
	}()

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
	// #nosec G114: This is just a test
	err = http.ListenAndServe(":8080", handler)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Failed to run server", "error", err)
		os.Exit(1)
	}
}
