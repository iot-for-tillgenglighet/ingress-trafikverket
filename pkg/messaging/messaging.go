package messaging

type IoTHubMessageOrigin struct {
	Device    string  `json:"device"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type IoTHubMessage struct {
	Origin    IoTHubMessageOrigin `json:"origin"`
	Timestamp string              `json:"timestamp"`
}

var IoTHubCommandExchange = "iot-cmd-exchange-direct"
var IoTHubTopicExchange = "iot-msg-exchange-topic"
