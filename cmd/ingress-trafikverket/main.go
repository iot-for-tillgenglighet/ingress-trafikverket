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

// TFVError encapsulates a lower level error together with an error
// message provided by the caller that experienced the error
type TFVError struct {
	msg string
	err error
}

// FatalTFVError signals that an unrecoverable error has occured and that
// the calling application should terminate
type FatalTFVError struct {
	TFVError
}

// Error returns a concatenated error string
func (err *TFVError) Error() string {
	if err.err != nil {
		return err.msg + " (" + err.err.Error() + ")"
	}

	return err.msg
}

// NewError returns a new TFVError instance
func NewError(msg string, err error) *TFVError {
	return &TFVError{msg, err}
}

func (err *FatalTFVError) Error() string {
	return "FATAL: " + err.TFVError.Error()
}

// NewFatalError returns a new FatalTFVError instance
func NewFatalError(msg string, err error) *FatalTFVError {
	return &FatalTFVError{
		TFVError: TFVError{msg, err},
	}
}

// MessageTopicInterface uses interface segregation to allow simpler mocking
// of code that wants to publish messages on topics
type MessageTopicInterface interface {
	PublishOnTopic(msg messaging.TopicMessage) error
}

type geometry struct {
	Position string `json:"WGS84"`
}

type measurement struct {
	Air         air    `json:"Air"`
	MeasureTime string `json:"MeasureTime"`
}

type air struct {
	Temp float64 `json:"Temp"`
}

type weatherStation struct {
	ID          string      `json:"ID"`
	Name        string      `json:"Name"`
	Geometry    geometry    `json:"Geometry"`
	Measurement measurement `json:"Measurement"`
}
type weatherStationResponse struct {
	Response struct {
		Result []struct {
			WeatherStations []weatherStation `json:"WeatherStation"`
			Info            struct {
				LastChangeID string `json:"LASTCHANGEID"`
			} `json:"INFO"`
		} `json:"RESULT"`
	} `json:"RESPONSE"`
}

// TrafikverketURL holds the URL to be called when getting data from Trafikverket
var TrafikverketURL string = "https://api.trafikinfo.trafikverket.se/v2/data.json"

func getAndPublishWeatherStationStatus(authKey string, lastChangeID string, messenger MessageTopicInterface) (string, error) {

	responseBody, err := getWeatherStationStatus(authKey, lastChangeID)

	answer := &weatherStationResponse{}
	err = json.Unmarshal(responseBody, answer)
	if err != nil {
		return lastChangeID, NewError("Unable to marshal response", err)
	}

	for _, weatherstation := range answer.Response.Result[0].WeatherStations {
		err = publishWeatherStationStatus(weatherstation, messenger)
		if err != nil {
			return lastChangeID, NewError("Unable to marshal response", err)
		}
	}

	return answer.Response.Result[0].Info.LastChangeID, nil
}

func publishWeatherStationStatus(weatherstation weatherStation, messenger MessageTopicInterface) error {

	position := weatherstation.Geometry.Position
	position = position[7 : len(position)-1]

	Longitude := strings.Split(position, " ")[0]
	newLong, _ := strconv.ParseFloat(Longitude, 32)
	Latitude := strings.Split(position, " ")[1]
	newLat, _ := strconv.ParseFloat(Latitude, 32)

	ts, err := time.Parse(time.RFC3339, weatherstation.Measurement.MeasureTime)

	if err != nil {
		log.Error("Failed to parse timestamp " + weatherstation.Measurement.MeasureTime)
		// continue
	}

	timeStamp := ts.UTC().Format(time.RFC3339)

	message := &telemetry.Temperature{
		IoTHubMessage: messaging.IoTHubMessage{
			Origin: messaging.IoTHubMessageOrigin{
				Device:    weatherstation.ID,
				Latitude:  newLat,
				Longitude: newLong,
			},
			Timestamp: timeStamp,
		},
		Temp: weatherstation.Measurement.Air.Temp,
	}

	return messenger.PublishOnTopic(message)
}

func getWeatherStationStatus(authKey string, lastChangeID string) ([]byte, error) {
	requestBody := fmt.Sprintf("<REQUEST><LOGIN authenticationkey=\"%s\" /><QUERY objecttype=\"WeatherStation\" schemaversion=\"1\" changeid=\"%s\"><INCLUDE>Id</INCLUDE><INCLUDE>Geometry.WGS84</INCLUDE><INCLUDE>Measurement.Air.Temp</INCLUDE><INCLUDE>Measurement.MeasureTime</INCLUDE><INCLUDE>ModifiedTime</INCLUDE><INCLUDE>Name</INCLUDE><FILTER><WITHIN name=\"Geometry.SWEREF99TM\" shape=\"box\" value=\"527000 6879000, 652500 6950000\" /></FILTER></QUERY></REQUEST>", authKey, lastChangeID)

	apiResponse, err := http.Post(
		TrafikverketURL,
		"text/xml",
		bytes.NewBufferString(requestBody),
	)

	if err != nil {
		return []byte(lastChangeID), NewError("Failed to request weather station data from Trafikverket", err)
	}

	defer apiResponse.Body.Close()

	responseBody, err := ioutil.ReadAll(apiResponse.Body)

	log.Info("Received response: " + string(responseBody))

	return responseBody, err
}

func main() {

	serviceName := "ingress-trafikverket"

	log.Infof("Starting up %s ...", serviceName)

	authKeyEnvironmentVariable := "TFV_API_AUTH_KEY"
	authenticationKey := os.Getenv(authKeyEnvironmentVariable)

	if authenticationKey == "" {
		log.Fatal("API authentication key missing. Please set " + authKeyEnvironmentVariable + " to a valid API key.")
	}

	config := messaging.LoadConfiguration(serviceName)
	messenger, _ := messaging.Initialize(config)

	defer messenger.Close()

	lastChangeID := "0"
	var err error = nil

	for {
		lastChangeID, err = getAndPublishWeatherStationStatus(authenticationKey, lastChangeID, messenger)
		if err != nil {
			switch err.(type) {
			case *FatalTFVError:
				log.Fatal(err)
				break
			default:
				log.Error(err)
			}
		}
		time.Sleep(30 * time.Second)
	}
}
