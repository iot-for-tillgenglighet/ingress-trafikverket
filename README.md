# Introduction 

This service is responsible for periodically retrieving data from the API library provided by Trafikverket.. 

## Deprecation Notice

This repository is deprecated and the code has moved to https://github.com/diwise/ingress-trafikverket

# Getting Started

In order to be able to use this service an API authentication key is needed. To prevent denial of service between developers, it is recommended that each developer has a unique authentication key. It is however possible to register these keys under a common account at Trafikverket. Please refer to https://api.trafikinfo.trafikverket.se for details.

# Building and tagging with Docker

`docker build -f deployments/Dockerfile -t iot-for-tillgenglighet/ingress-trafikverket:latest .`

# Build for local testing with Docker Compose

`docker-compose -f ./deployments/docker-compose.yml build`

# Running locally with Docker Compose

`TFV_API_AUTH_KEY=<insert your API key here> docker-compose -f ./deployments/docker-compose.yml up`

The ingress service will exit fatally and restart a couple of times until the RabbitMQ container is properly initialized and ready to accept connections. This is to be expected.

To clean up the environment properly after testing it is advisable to run `docker-compose down -v`
