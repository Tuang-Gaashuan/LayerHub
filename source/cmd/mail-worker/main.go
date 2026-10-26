package main

import (
	"context"
	"layerhub.local/server/internal/mailer"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	token, err := os.ReadFile("/etc/layerhub-mail/relay.token")
	if err != nil {
		log.Fatal("mail relay token unavailable")
	}
	w, err := mailer.New(strings.TrimSpace(string(token)), os.Getenv("LAYERHUB_PUBLIC_URL"), "/etc/layerhub-mail/smtp.json", "/var/lib/layerhub-mail/state.json")
	if err != nil {
		log.Fatal("mail worker configuration invalid")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	w.Run(ctx)
}
