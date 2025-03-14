package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	Config    AppConfig  `yaml:"config"`
	Services  []Service  `yaml:"services"`
	Ports     []Port     `yaml:"ports"`
	Endpoints []Endpoint `yaml:"endpoints"`
}

type AppConfig struct {
	Listen struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"listen"`
	SSL struct {
		Enabled  bool   `yaml:"enabled"`
		CertFile string `yaml:"certFile"`
		KeyFile  string `yaml:"keyFile"`
	} `yaml:"ssl"`
	Auth struct {
		Enabled  bool   `yaml:"enabled"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"auth"`
}

type Service struct {
	Name   string `yaml:"name"`
	Status string `yaml:"status"`
}

type Port struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

type Endpoint struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Status   int    `yaml:"status"`
	Statuses []int  `yaml:"statuses"`
}

var outputMessages []string

func main() {
	configFilePath := flag.String("config", GetEnv("HEALTHCHECK_CONFIG_FILE", "config.yaml"), "Path to the config file")

	flag.Parse()

	config, err := readConfig(*configFilePath)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	http.HandleFunc("/healthy", basicAuthMiddleware(config.Config.Auth, func(w http.ResponseWriter, r *http.Request) {
		outputMessages = []string{} // Reinitialize outputMessages
		response := make(map[string]interface{})
		if !checkPorts(config.Ports) || !checkServices(config.Services) || !checkEndpoints(config.Endpoints) {
			w.WriteHeader(http.StatusInternalServerError)
			response["status"] = "Server is unhealthy"
		} else {
			w.WriteHeader(http.StatusOK)
			response["status"] = "Server is healthy"
		}
		response["messages"] = outputMessages
		json.NewEncoder(w).Encode(response)
	}))

	l := fmt.Sprintf("%s:%d", GetEnv("HEALTH_LISTEN_HOST", config.Config.Listen.Host), GetEnvInt("HEALTH_LISTEN_PORT", config.Config.Listen.Port))
	log.Printf("Starting server on %s", l)
	var serverErr error
	if config.Config.SSL.Enabled {
		serverErr = http.ListenAndServeTLS(l, config.Config.SSL.CertFile, config.Config.SSL.KeyFile, nil)
	} else {
		serverErr = http.ListenAndServe(l, nil)
	}

	if serverErr != nil {
		log.Fatalf("Failed to start server: %v", serverErr)
	}
}

func basicAuthMiddleware(authConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authConfig.Enabled {
			username, password, ok := r.BasicAuth()
			if !ok || username != authConfig.Username || password != authConfig.Password {
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func readConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func checkServices(services []Service) bool {
	var err_count int
	for _, service := range services {
		cmd := exec.Command("systemctl", "is-active", service.Name)
		output, err := cmd.Output()
		status := strings.TrimSpace(string(output))
		if err != nil || status != service.Status {
			addToOutputMessages("Service Name: %s, Expected Status: %s, Actual Status: %s", service.Name, service.Status, status)
			err_count++
		} else {
			addToOutputMessages("Service Name: %s, Status: %s is as expected", service.Name, service.Status)
		}
	}
	return err_count == 0
}

func checkPorts(ports []Port) bool {
	var err_count int
	for _, port := range ports {
		address := fmt.Sprintf("%s:%d", port.Address, port.Port)
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err != nil {
			addToOutputMessages("Port Name: %s, Port: %d is not available", port.Name, port.Port)
			err_count++
		} else {
			addToOutputMessages("Port Name: %s, Port: %d is available", port.Name, port.Port)
			conn.Close()
		}
	}
	return err_count == 0
}

func checkEndpoints(endpoints []Endpoint) bool {
	var err_count int
	for _, endpoint := range endpoints {
		var resp *http.Response
		var err error

		if strings.HasPrefix(endpoint.URL, "https://") {
			// Disable SSL verification
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			client := &http.Client{Transport: tr}
			resp, err = client.Get(endpoint.URL)
		} else {
			resp, err = http.Get(endpoint.URL)
		}

		if err != nil {
			addToOutputMessages("Endpoint Name: %s, URL: %s is not reachable", endpoint.Name, endpoint.URL)
			err_count++
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == endpoint.Status || contains(endpoint.Statuses, resp.StatusCode) {
			addToOutputMessages("Endpoint Name: %s, URL: %s, Status: %d is as expected", endpoint.Name, endpoint.URL, endpoint.Status)
		} else {
			addToOutputMessages("Endpoint Name: %s, URL: %s, Status: %d is not as expected, got: %d", endpoint.Name, endpoint.URL, endpoint.Status, resp.StatusCode)
			err_count++
		}
	}
	return err_count == 0
}

func addToOutputMessages(format string, a ...interface{}) {
	message := fmt.Sprintf(format, a...)
	outputMessages = append(outputMessages, message)
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		} else {
			log.Fatalf("Invalid value for %s: %s", key, value)
		}
	}
	return fallback
}

func contains(numbers []int, target int) bool {
	for _, num := range numbers {
		if num == target {
			return true
		}
	}
	return false
}
