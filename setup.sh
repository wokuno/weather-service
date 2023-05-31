#!/bin/bash

# Generate a random password for PostgreSQL
POSTGRES_PASSWORD=$(openssl rand -base64 12)

# Create the user weather
useradd -m -s /bin/bash weather
usermod -aG wheel weather

# Copy authorized_keys from root to weather's home directory
cp /root/.ssh/authorized_keys /home/weather/.ssh/
chown weather:weather /home/weather/.ssh/authorized_keys

# Update the system and install dependencies
dnf update -y
dnf install -y curl wget git

# Install Podman
dnf -y install podman
systemctl start podman
systemctl enable podman

# Install Podman Compose
sudo dnf install -y podman-compose

# Set up Caddy and PostgreSQL containers (replace with your desired configuration)
mkdir -p /srv/caddy
mkdir -p /srv/pgdata

# Clone the GitHub repository
git clone https://github.com/wokuno/weather-service /tmp/weather-service

# Copy the Caddyfile and Docker Compose file from the repository
cp /tmp/weather-service/Caddyfile /srv/
cp /tmp/weather-service/docker-compose.yaml /srv/

# Change ownership of the directories
chown -R weather:weather /srv/caddy
chown -R weather:weather /srv/pgdata

# Set the environment variables
echo "export POSTGRES_USER=weather" >> /home/weather/.bashrc
echo "export POSTGRES_PASSWORD=$POSTGRES_PASSWORD" >> /home/weather/.bashrc
echo "export POSTGRES_DB=weatherdb" >> /home/weather/.bashrc

# Start the containers using Podman Compose
cd /srv
sudo -u weather podman-compose up -d
