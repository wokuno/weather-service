# Use the official Golang image as the base image
FROM golang:latest

# Set the working directory inside the container
WORKDIR /app

# Copy the source code files from the host to the container
COPY . /app

# Download the files from the GitHub repository
RUN apt-get update && apt-get install -y git && \
    git clone https://github.com/wokuno/weather-service.git /tmp/weather-service && \
    cp -R /tmp/weather-service/* /app/ && \
    rm -rf /tmp/weather-service && \
    apt-get remove -y git && apt-get autoremove -y && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Build the Go application
RUN go build -o main .

# Expose the port that the Go application listens on
EXPOSE 8080

# Run the Go application when the container starts
CMD ["./main"]
