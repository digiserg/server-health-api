# Server Health API

**NOTE:** This project was entirely created using Copilot Edits as a Proof of Concept

This Go application checks the health of various services, ports, and endpoints on a server. It can be used to verify whether a server is healthy before performing maintenance on it.

## Features

- Check the status of systemd services.
- Verify the availability of TCP ports.
- Perform HTTP requests to endpoints and check their status codes.
- Supports SSL and basic authentication for endpoints.
- Configurable via a YAML file.

## Configuration

The application is configured using a `config.yaml` file. Below is an example configuration:

```yaml
config:
  listen:
    host: "0.0.0.0"
    port: 8080
  ssl:
    enabled: false
    certFile: "path/to/certfile"
    keyFile: "path/to/keyfile"
  auth:
    enabled: false
    username: "user"
    password: "pass"
services:
  - name: "nginx"
    status: "active"
ports:
  - name: "HTTP"
    address: "127.0.0.1"
    port: 80
endpoints:
  - name: "Google"
    url: "https://www.google.com"
    status: 200
    # alternatively, use a list
    #statuses: [200, 301]
```

## Running the Application

### Using Go

1. Build the application:

    ```sh
    make build
    ```

2. Run the application:

    ```sh
    make run
    ```

### Using Docker

1. Build the Docker image:

    ```sh
    docker build -t server-health-api .
    ```

2. Run the Docker container:

    ```sh
    docker run -p 8080:8080 -e HEALTH_LISTEN_HOST="0.0.0.0" -e HEALTH_LISTEN_PORT="8080" server-health-api
    ```

## API Endpoint

The application exposes a single endpoint:

- `GET /healthy`: Checks the health of the configured services, ports, and endpoints. Returns a JSON response with the status and messages.

Example response:

```json
{
  "status": "Server is healthy",
  "messages": [
    "Service Name: nginx, Status: active is as expected",
    "Port Name: HTTP, Port: 80 is available",
    "Endpoint Name: Google, URL: https://www.google.com, Status: 200 is as expected"
  ]
}
```

## Environment Variables

- `HEALTH_LISTEN_HOST`: The host address to listen on (default: `0.0.0.0`).
- `HEALTH_LISTEN_PORT`: The port to listen on (default: `8080`).
- `HEALTHCHECK_CONFIG_FILE`: The path to the configuration file (default: `config.yaml`).

## Command Line Options

- `-config`: Specify the path to the configuration file. This overrides the `HEALTHCHECK_CONFIG_FILE` environment variable.

## License

This project is licensed under the Apache License.
