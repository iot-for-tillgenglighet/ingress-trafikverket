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

	"github.com/iot-for-tillgenglighet/ingress-trafikverket/pkg/messaging"
	"github.com/iot-for-tillgenglighet/ingress-trafikverket/pkg/messaging/telemetry"

	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
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

	answer := &weatherStationResponse{}
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

		responseBody, err := json.MarshalIndent(message, "", " ")
		if err != nil {
			log.Fatal("Marshal problem: " + err.Error())
		}

		err = exchange.Publish(messaging.IoTHubTopicExchange, telemetry.TemperatureTopic, false, false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        responseBody,
			})

		if err != nil {
			log.Fatal("Failed to publish telemetry message to topic: " + err.Error())
		}
	}

	return answer.Response.Result[0].Info.LastChangeID
}

func createMessageQueueChannel(host, user, password string) (*amqp.Connection, *amqp.Channel) {
	connectionString := fmt.Sprintf("amqp://%s:%s@%s:5672/", user, password, host)
	conn, err := amqp.Dial(connectionString)
	if err != nil {
		log.Fatal("Unable to connect to message queue: " + err.Error())
	}

	amqpChannel, err := conn.Channel()

	if err != nil {
		log.Fatal("Unable to create an amqp channel to message queue: " + err.Error())
	}

	return conn, amqpChannel
}

func createCommandExchangeOrDie(amqpChannel *amqp.Channel) string {
	err := amqpChannel.ExchangeDeclare(messaging.IoTHubCommandExchange, amqp.ExchangeDirect, false, false, false, false, nil)

	if err != nil {
		log.Fatal("Unable to declare command exchange " + messaging.IoTHubCommandExchange + ": " + err.Error())
	}

	return messaging.IoTHubCommandExchange
}

func createTopicExchangeOrDie(amqpChannel *amqp.Channel) string {
	err := amqpChannel.ExchangeDeclare(messaging.IoTHubTopicExchange, amqp.ExchangeTopic, false, false, false, false, nil)

	if err != nil {
		log.Fatal("Unable to declare exchange " + messaging.IoTHubTopicExchange + ": " + err.Error())
	}

	return messaging.IoTHubTopicExchange
}

func createCommandAndResponseQueues(amqpChannel *amqp.Channel) {
	exchangeName := createCommandExchangeOrDie(amqpChannel)

	commandQueue, err := amqpChannel.QueueDeclare("ingress-trafikverket", false, false, false, false, nil)
	if err != nil {
		log.Fatal("Failed to declare command queue for ingress-trafikverket!")
	}

	err = amqpChannel.QueueBind(commandQueue.Name, "ingress-trafikverket", exchangeName, false, nil)
	if err != nil {
		log.Fatalf("Failed to bind command queue %s to exchange %s!", commandQueue.Name, exchangeName)
	}

	responseQueue, err := amqpChannel.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		log.Fatal("Failed to declare response queue for ingress-trafikverket!")
	}

	err = amqpChannel.QueueBind(responseQueue.Name, responseQueue.Name, exchangeName, false, nil)
	if err != nil {
		log.Fatalf("Failed to bind response queue %s to exchange %s!", responseQueue.Name, exchangeName)
	}

	commands, err := amqpChannel.Consume(commandQueue.Name, "command-consumer", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Unable to start consuming commands from %s!", commandQueue.Name)
	}

	go func() {
		for cmd := range commands {
			log.Info("Received command: " + string(cmd.Body))

			err = amqpChannel.Publish(exchangeName, cmd.ReplyTo, true, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte("{\"cmd\": \"pong\"}"),
			})
			if err != nil {
				log.Error("Failed to publish a pong response to ourselves!")
			}

			cmd.Ack(true)
		}
	}()

	responses, err := amqpChannel.Consume(responseQueue.Name, "response-consumer", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Unable to start consuming responses from %s!", responseQueue.Name)
	}

	err = amqpChannel.Publish(exchangeName, "ingress-trafikverket", true, false, amqp.Publishing{
		ContentType: "application/json",
		ReplyTo:     responseQueue.Name,
		Body:        []byte("{\"cmd\": \"ping\"}"),
	})
	if err != nil {
		log.Fatal("Failed to publish a ping command to ourselves!")
	}

	go func() {
		for response := range responses {
			log.Info("Received response: " + string(response.Body))
			response.Ack(true)
		}
	}()

	// TODO: Replace with a barrier that waits for the ping<->pong to complete
	time.Sleep(5 * time.Second)
}

func initializeMessaging(host, user, password string) (*amqp.Connection, *amqp.Channel) {
	conn, channel := createMessageQueueChannel(host, user, password)

	createTopicExchangeOrDie(channel)
	createCommandAndResponseQueues(channel)

	return conn, channel
}

func main() {

	log.Info("Starting up ingress-trafikverket ...")

	authKeyEnvironmentVariable := "TFV_API_AUTH_KEY"
	authenticationKey := os.Getenv(authKeyEnvironmentVariable)

	if authenticationKey == "" {
		log.Fatal("API authentication key missing. Please set " + authKeyEnvironmentVariable + " to a valid API key.")
	}

	conn, channel := initializeMessaging("rabbitmq", "user", "bitnami")

	defer conn.Close()
	defer channel.Close()

	lastChangeID := "0"

	for {
		if conn.IsClosed() {
			log.Error("Connection is closed!")
		}

		lastChangeID = getAndPublishWeatherStationStatus(authenticationKey, lastChangeID, channel)
		time.Sleep(30 * time.Second)
	}
}
