package telemetry

import (
	"context"
	"net/http"
	"strings"

	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/ipni/storetheindex/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// SetupTelemetry configures OpenTelemetry via passed config or environment variables.
//
// https://docs.honeycomb.io/send-data/go/opentelemetry-sdk/#configure-environment-variables
func SetupTelemetry(cfg *config.Config) (func(), error) {
	var opts []otelconfig.Option
	if cfg.OTELServiceName != "" {
		opts = append(opts, otelconfig.WithServiceName(cfg.OTELServiceName))
	}
	if cfg.OTELExporterEndpoint != "" {
		opts = append(opts, otelconfig.WithServiceName(cfg.OTELExporterEndpoint))
	}
	if cfg.OTELExporterHeaders != "" {
		headers := map[string]string{}
		for h := range strings.SplitSeq(cfg.OTELExporterHeaders, ",") {
			kv := strings.Split(h, "=")
			if len(kv) < 2 {
				continue
			}
			headers[kv[0]] = kv[1]
		}
		opts = append(opts, otelconfig.WithHeaders(headers))
	}
	if cfg.OTELSamplerRatio != 0 {
		opts = append(opts, otelconfig.WithSampler(sdktrace.TraceIDRatioBased(cfg.OTELSamplerRatio)))
	}
	return otelconfig.ConfigureOpenTelemetry(opts...)
}

func InstrumentHTTPClient(client *http.Client) *http.Client {
	instrumentedTransport := otelhttp.NewTransport(client.Transport)
	client.Transport = instrumentedTransport
	return client
}

func InstrumentHTTPHandler(handler http.HandlerFunc, operation string) http.HandlerFunc {
	return otelhttp.NewHandler(handler, operation).ServeHTTP
}

func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	t := otel.Tracer("storetheindex")
	return t.Start(ctx, name)
}

func Error(span trace.Span, err error, msg string) {
	span.SetStatus(codes.Error, msg)
	span.RecordError(err)
}
