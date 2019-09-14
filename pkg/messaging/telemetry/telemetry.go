package telemetry

import "github.com/iot-for-tillgenglighet/ingress-trafikverket/pkg/messaging"

type Temperature struct {
	messaging.IoTHubMessage
	Temp float32 `json:"temp"`
}

func (msg *Temperature) TopicName() string {
	return "telemetry.temperature"
}
