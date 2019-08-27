# Introduction 

This service is responsible for periodically retrieving data from the API library provided by Trafikverket.. 

# Getting Started

In order to be able to use this service an API authentication key is needed. To prevent denial of service between developers, it is recommended that each developer has a unique authentication key. It is however possible to register these keys under a common account at Trafikverket. Please refer to https://api.trafikinfo.trafikverket.se for details.

# Building with Docker

docker build -f deployments/Dockerfile -t iot-for-tillgenglighet/ingress-trafikverket:latest .

# Running with Docker

docker run -it -e TFV_API_AUTH_KEY='<insert your API key here>' iot-for-tillgenglighet/ingress-trafikverket

