# Use the official Golang image as the base image
FROM golang:1.23-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o server-health-api

# Start a new stage from scratch
FROM alpine:latest

ENV HEALTH_LISTEN_HOST="0.0.0.0"
ENV HEALTH_LISTEN_PORT="8080"

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/server-health-api /server-health-api

# Command to run the executable
CMD ["/server-health-api"]
