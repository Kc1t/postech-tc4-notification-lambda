// Lambda de notificação: SNS os-events → SQS notification-os-events → e-mail ao cliente.
package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/Kc1t/postech-tc4-notification-lambda/internal/idempotency"
	"github.com/Kc1t/postech-tc4-notification-lambda/internal/mail"
	"github.com/Kc1t/postech-tc4-notification-lambda/internal/notifier"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/newrelic/go-agent/v3/integrations/nrlambda"
	"github.com/newrelic/go-agent/v3/newrelic"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level(env("LOG_LEVEL", "info"))}))
	slog.SetDefault(log)

	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Error("aws config", "error", err)
		os.Exit(1)
	}
	ttl, _ := time.ParseDuration(env("DEDUP_TTL", "168h"))
	timeout, _ := time.ParseDuration(env("SMTP_TIMEOUT", "10s"))
	port, _ := strconv.Atoi(env("SMTP_PORT", "587"))

	h := notifier.New(
		&mail.SMTP{
			Host: os.Getenv("SMTP_HOST"), Port: strconv.Itoa(port),
			Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"),
			From: env("SMTP_FROM", "noreply@reservoirdevs.online"), FromName: env("SMTP_FROM_NAME", "Oficina Reservoir Devs"),
			Timeout: timeout,
		},
		idempotency.NewDynamoStore(dynamodb.NewFromConfig(awsCfg), env("DEDUP_TABLE", "notification-dedup"), ttl),
		os.Getenv("PORTAL_URL"),
		log,
	)

	// Com a layer NewRelicLambdaExtension, o nrlambda entrega a telemetria pela extension.
	app, err := newrelic.NewApplication(nrlambda.ConfigOption())
	if err != nil {
		log.Warn("new relic desabilitado", "error", err)
		lambda.Start(h.Handle)
		return
	}
	nrlambda.Start(h.Handle, app)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func level(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}
