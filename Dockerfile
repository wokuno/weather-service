# Use the official Golang image as the base image
FROM golang:latest

# Set the working directory inside the container
WORKDIR /app

# Copy the source code files from the host to the container
COPY . /app

# Build the Go application
RUN go build -o main .

# Expose the port that the Go application listens on
EXPOSE 8080

# Run the Go application when the container starts
CMD ["./main"]
