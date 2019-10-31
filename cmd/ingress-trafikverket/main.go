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

	"github.com/iot-for-tillgenglighet/messaging-golang/pkg/messaging"
	"github.com/iot-for-tillgenglighet/messaging-golang/pkg/messaging/telemetry"

	log "github.com/sirupsen/logrus"
)

type weatherStationResponse struct {
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
						Temp float64 `json:"Temp"`
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

func getAndPublishWeatherStationStatus(authKey string, lastChangeID string, messenger *messaging.Context) string {

	requestBody := fmt.Sprintf("<REQUEST><LOGIN authenticationkey=\"%s\" /><QUERY objecttype=\"WeatherStation\" schemaversion=\"1\" changeid=\"%s\"><INCLUDE>Id</INCLUDE><INCLUDE>Geometry.WGS84</INCLUDE><INCLUDE>Measurement.Air.Temp</INCLUDE><INCLUDE>Measurement.MeasureTime</INCLUDE><INCLUDE>ModifiedTime</INCLUDE><INCLUDE>Name</INCLUDE><FILTER><WITHIN name=\"Geometry.SWEREF99TM\" shape=\"box\" value=\"527000 6879000, 652500 6950000\" /></FILTER></QUERY></REQUEST>", authKey, lastChangeID)

	apiResponse, err := http.Post(
		"https://api.trafikinfo.trafikverket.se/v2/data.json",
		"text/xml",
		bytes.NewBufferString(requestBody),
	)

	if err != nil {
		log.Fatal("Failed to request weather station data from Trafikverket: " + err.Error())
	}

	defer apiResponse.Body.Close()

	responseBody, err := ioutil.ReadAll(apiResponse.Body)

	log.Info("Received response: " + string(responseBody))

	answer := &weatherStationResponse{}
	err = json.Unmarshal(responseBody, answer)
	if err != nil {
		log.Error("Unable to unmarshal response: " + err.Error())
		return lastChangeID
	}

	for _, weatherstation := range answer.Response.Result[0].WeatherStations {

		position := weatherstation.Geometry.Position
		position = position[7 : len(position)-1]

		Latitude := strings.Split(position, " ")[0]
		newLat, err := strconv.ParseFloat(Latitude, 32)
		Longitude := strings.Split(position, " ")[1]
		newLong, err := strconv.ParseFloat(Longitude, 32)

		message := &telemetry.Temperature{
			IoTHubMessage: messaging.IoTHubMessage{
				Origin: messaging.IoTHubMessageOrigin{
					Device:    weatherstation.ID,
					Latitude:  newLat,
					Longitude: newLong,
				},
				Timestamp: weatherstation.Measurement.MeasureTime,
			},
			Temp: weatherstation.Measurement.Air.Temp,
		}

		err = messenger.PublishOnTopic(message)

		if err != nil {
			log.Fatal("Failed to publish telemetry message to topic: " + err.Error())
		}
	}

	return answer.Response.Result[0].Info.LastChangeID
}

func main() {

	serviceName := "ingress-trafikverket"

	log.Infof("Starting up %s ...", serviceName)

	authKeyEnvironmentVariable := "TFV_API_AUTH_KEY"
	authenticationKey := os.Getenv(authKeyEnvironmentVariable)

	if authenticationKey == "" {
		log.Fatal("API authentication key missing. Please set " + authKeyEnvironmentVariable + " to a valid API key.")
	}

	rabbitMQHostEnvVar := "RABBITMQ_HOST"
	rabbitMQHost := os.Getenv(rabbitMQHostEnvVar)
	rabbitMQUser := os.Getenv("RABBITMQ_USER")
	rabbitMQPass := os.Getenv("RABBITMQ_PASS")

	if rabbitMQHost == "" {
		log.Fatal("Rabbit MQ host missing. Please set " + rabbitMQHostEnvVar + " to a valid host name or IP.")
	}

	var messenger *messaging.Context
	var err error

	for messenger == nil {

		time.Sleep(2 * time.Second)

		messenger, err = messaging.Initialize(messaging.Config{
			ServiceName: serviceName,
			Host:        rabbitMQHost,
			User:        rabbitMQUser,
			Password:    rabbitMQPass,
		})

		if err != nil {
			log.Error(err)
		}
	}

	defer messenger.Close()

	lastChangeID := "0"

	for {
		lastChangeID = getAndPublishWeatherStationStatus(authenticationKey, lastChangeID, messenger)
		time.Sleep(30 * time.Second)
	}
}
