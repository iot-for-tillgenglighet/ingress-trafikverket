package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
)

type WeatherStationResponse struct {
	Response struct {
		Result []struct {
			WeatherStations []struct {
				ID       string `json:"ID"`
				Name     string `json:"Name"`
				Geometry struct {
					Position string `json:"WGS84"`
				} `json:"Geometry"`
				Measurement struct {
					Air struct {
						Temp float32 `json:"Temp"`
					} `json:"Air"`
					MeasureTime string `json:"MeasureTime"`
				} `json:"Measurement"`
			} `json:"WeatherStation"`
			Info struct {
				LastChangeID string `json:"LASTCHANGEID"`
			} `json:"INFO"`
		} `json:"RESULT"`
	} `json:"RESPONSE"`
}

type IoTHubMessageOrigin struct {
	Device    string  `json:"device"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type IoTHubMessage struct {
	Origin    IoTHubMessageOrigin `json:"origin"`
	Timestamp string              `json:"timestamp"`
}

type TelemetryTemperature struct {
	IoTHubMessage
	Temp float32 `json:"temp"`
}

func getAndPublishWeatherStationStatus(authKey string, lastChangeID string, exchange *amqp.Channel) string {

	requestBody := fmt.Sprintf("<REQUEST><LOGIN authenticationkey=\"%s\" /><QUERY objecttype=\"WeatherStation\" schemaversion=\"1\" changeid=\"%s\"><INCLUDE>Id</INCLUDE><INCLUDE>Geometry.WGS84</INCLUDE><INCLUDE>Measurement.Air.Temp</INCLUDE><INCLUDE>Measurement.MeasureTime</INCLUDE><INCLUDE>ModifiedTime</INCLUDE><INCLUDE>Name</INCLUDE><FILTER><WITHIN name=\"Geometry.SWEREF99TM\" shape=\"box\" value=\"527000 6879000, 652500 6950000\" /></FILTER></QUERY></REQUEST>", authKey, lastChangeID)

	apiResponse, err := http.Post(
		"https://api.trafikinfo.trafikverket.se/v2/data.json",
		"text/xml",
		bytes.NewBufferString(requestBody),
	)

	if err != nil {
		errorString := fmt.Sprintf("Failed to request weather station data from Trafikverket: %s", err)
		log.Fatal(errorString)
	}

	defer apiResponse.Body.Close()

	responseBody, err := ioutil.ReadAll(apiResponse.Body)

	log.Info("Received response: " + string(responseBody))

	answer := &WeatherStationResponse{}
	err = json.Unmarshal(responseBody, answer)
	if err != nil {
		log.Fatal("Unmarshal problem")
	}

	for _, weatherstation := range answer.Response.Result[0].WeatherStations {

		position := weatherstation.Geometry.Position
		position = position[7 : len(position)-1]

		Latitude := strings.Split(position, " ")[0]
		newLat, err := strconv.ParseFloat(Latitude, 32)
		Longitude := strings.Split(position, " ")[1]
		newLong, err := strconv.ParseFloat(Longitude, 32)

		message := &TelemetryTemperature{
			IoTHubMessage: IoTHubMessage{
				Origin: IoTHubMessageOrigin{
					Device:    weatherstation.ID,
					Latitude:  newLat,
					Longitude: newLong,
				},
				Timestamp: weatherstation.Measurement.MeasureTime,
			},
			Temp: weatherstation.Measurement.Air.Temp,
		}

		responseBody, err := json.MarshalIndent(message, "", " ")
		if err != nil {
			log.Fatal("Marshal problem: " + err.Error())
		}

		err = exchange.Publish("iot-msg-exchange-topic", "telemetry.temperature", false, false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        responseBody,
			})

		if err != nil {
			log.Error("Failed to publish telemetry message to topic: " + err.Error())
		}
	}

	return answer.Response.Result[0].Info.LastChangeID
}

func createMessageQueueChannel() (*amqp.Connection, *amqp.Channel) {
	conn, err := amqp.Dial("amqp://user:bitnami@rabbitmq:5672/")
	if err != nil {
		log.Fatal("Unable to connect to message queue: " + err.Error())
	}

	amqpChannel, err := conn.Channel()

	if err != nil {
		log.Fatal("Unable to create an amqp channel to message queue: " + err.Error())
	}

	exchangeName := "iot-msg-exchange-topic"

	err = amqpChannel.ExchangeDeclare(exchangeName, amqp.ExchangeTopic, false, false, false, false, nil)

	if err != nil {
		log.Fatal("Unable to declare exchange " + exchangeName + ": " + err.Error())
	}

	return conn, amqpChannel
}

func main() {

	authKeyEnvironmentVariable := "TFV_API_AUTH_KEY"
	authenticationKey := os.Getenv(authKeyEnvironmentVariable)

	if authenticationKey == "" {
		log.Fatal("API authentication key missing. Please set " + authKeyEnvironmentVariable + " to a valid API key.")
	}

	conn, channel := createMessageQueueChannel()

	defer conn.Close()
	defer channel.Close()

	lastChangeID := "0"

	for {
		lastChangeID = getAndPublishWeatherStationStatus(authenticationKey, lastChangeID, channel)
		time.Sleep(30 * time.Second)
	}
}
