package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	doctor := flag.Bool("doctor", false, "Inspect deployment without starting a service or contacting devices")
	jsonOutput := flag.Bool("json", false, "Output diagnostic JSON")
	probe := flag.Bool("probe-write", false, "Doctor: create/read/remove one temporary file in the existing data directory")
	language := flag.String("language", "en", "Doctor language: en or zh-CN")
	flag.Parse()
	if flag.NArg() != 0 || (*probe || *jsonOutput) && !*doctor || *language != "en" && *language != "zh-CN" {
		fmt.Fprintln(os.Stderr, "Use --doctor [--json] [--probe-write] [--language=en|zh-CN]")
		os.Exit(2)
	}
	cfg := app.EnvConfig()
	if *doctor {
		report := app.LocalizeDiagnosticReport(app.LocalDiagnostics(cfg, *probe), *language)
		if *jsonOutput {
			_ = json.NewEncoder(os.Stdout).Encode(report)
		} else {
			fmt.Printf("LayerHub %s · %s\n", report.Version, report.Platform)
			for _, c := range report.Checks {
				fmt.Printf("[%s] %s: %s\n", c.Status, c.Label, c.Detail)
				if c.Fix != "" {
					fmt.Println("  " + c.Fix)
				}
			}
		}
		for _, c := range report.Checks {
			if c.Status == "error" {
				os.Exit(1)
			}
		}
		return
	}
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
