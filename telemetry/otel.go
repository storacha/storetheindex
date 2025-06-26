package telemetry

import (
	"context"
	"net/http"

	"github.com/honeycombio/otel-config-go/otelconfig"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SetupTelemetry configures OpenTelemetry via environment variables.
//
// https://docs.honeycomb.io/send-data/go/opentelemetry-sdk/#configure-environment-variables
func SetupTelemetry(opts ...otelconfig.Option) (func(), error) {
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
