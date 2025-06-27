package config

// Telemetry configures open telemetry for the application.
type Telemetry struct {
	ServiceName      string
	ExporterEndpoint string
	ExporterHeaders  string
	SamplerRatio     float64
}

// NewTelemetry returns Telemetry with values set to their defaults.
func NewTelemetry() Telemetry {
	return Telemetry{}
}
