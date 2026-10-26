package main

import (
	"context"
	"layerhub.local/server/internal/app"
	"layerhub.local/server/internal/device"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	cfg := app.EnvConfig()
	devices, err := device.New(cfg.DeviceMode)
	if err != nil {
		log.Fatal(err)
	}
	service, err := app.Open(cfg, devices)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: cfg.Address, Handler: service.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Printf("LayerHub Go: http://%s | device mode=%s | remote start=%v", cfg.Address, cfg.DeviceMode, cfg.EnablePrintStart)
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "bootstrap-admin.txt")); err == nil {
		log.Printf("Initial login: %s", filepath.Join(cfg.DataDir, "bootstrap-admin.txt"))
	}
	err = server.ListenAndServe()
	service.Close()
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
