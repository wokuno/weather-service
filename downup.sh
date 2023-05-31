#!/bin/bash

sudo git pull
podman build -t weather-service .
podman-compose down
podman-compose up -d