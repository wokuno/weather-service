#!/bin/bash

# Generate a random password for PostgreSQL
POSTGRES_PASSWORD=$(openssl rand -base64 12)

# Create the user wokuno
useradd -m -s /bin/bash wokuno
usermod -aG sudo wokuno

# Copy authorized_keys from root to wokuno's home directory
cp /root/.ssh/authorized_keys /home/wokuno/.ssh/
chown wokuno:wokuno /home/wokuno/.ssh/authorized_keys

# Update the system and install dependencies
apt-get update
apt-get upgrade -y
apt-get install -y apt-transport-https ca-certificates curl software-properties-common

# Install Podman
curl -sSL https://get.docker.com | sh
curl -sSL https://github.com/containers/podman/raw/main/installers/debian/podman.sh | sh

# Configure Podman to run as non-root user
groupadd podman
usermod -aG podman wokuno

# Set up Caddy and PostgreSQL containers (replace with your desired configuration)
mkdir -p /srv/caddy
mkdir -p /srv/pgdata

# Copy the Caddyfile and Docker Compose file from GitHub repository
git clone https://github.com/wokuno/weather-service /tmp/weather-service
cp /tmp/weather-service/Caddyfile /srv/
cp /tmp/weather-service/docker-compose.yaml /srv/

# Change ownership of the directories
chown -R wokuno:wokuno /srv/caddy
chown -R wokuno:wokuno /srv/pgdata

# Set the environment variables
echo "export POSTGRES_USER=weather" >> /home/wokuno/.bashrc
echo "export POSTGRES_PASSWORD=$POSTGRES_PASSWORD" >> /home/wokuno/.bashrc
echo "export POSTGRES_DB=weatherdb" >> /home/wokuno/.bashrc

# Start the containers using Podman and Docker Compose
cd /srv
sudo -u wokuno podman-compose up -d