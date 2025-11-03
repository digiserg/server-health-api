package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
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

func main() {
	configFilePath := flag.String("config", GetEnv("HEALTHCHECK_CONFIG_FILE", "config.yaml"), "Path to the config file")

	flag.Parse()

	config, err := readConfig(*configFilePath)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	http.HandleFunc("/healthy", basicAuthMiddleware(config.Config.Auth, func(w http.ResponseWriter, r *http.Request) {
		messages := []string{} // Local variable for this request
		response := make(map[string]interface{})
		if !checkPorts(config.Ports, &messages) || !checkServices(config.Services, &messages) || !checkEndpoints(config.Endpoints, &messages) {
			w.WriteHeader(http.StatusInternalServerError)
			response["status"] = "Server is unhealthy"
		} else {
			w.WriteHeader(http.StatusOK)
			response["status"] = "Server is healthy"
		}
		response["messages"] = messages
		json.NewEncoder(w).Encode(response)
	}))

	l := fmt.Sprintf("%s:%d", GetEnv("HEALTH_LISTEN_HOST", config.Config.Listen.Host), GetEnvInt("HEALTH_LISTEN_PORT", config.Config.Listen.Port))

	server := &http.Server{
		Addr:    l,
		Handler: nil,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on %s", l)
		var err error
		if config.Config.SSL.Enabled {
			err = server.ListenAndServeTLS(config.Config.SSL.CertFile, config.Config.SSL.KeyFile)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	log.Println("Server exited gracefully")
}

func basicAuthMiddleware(authConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authConfig.Enabled {
			username, password, ok := r.BasicAuth()
			userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(authConfig.Username)) == 1
			passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(authConfig.Password)) == 1
			if !ok || !userMatch || !passMatch {
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

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

func (c *Config) Validate() error {
	if c.Config.Listen.Port < 1 || c.Config.Listen.Port > 65535 {
		return fmt.Errorf("invalid listen port: %d", c.Config.Listen.Port)
	}
	for _, port := range c.Ports {
		if port.Port < 1 || port.Port > 65535 {
			return fmt.Errorf("invalid port: %d for %s", port.Port, port.Name)
		}
	}
	for _, endpoint := range c.Endpoints {
		if _, err := url.Parse(endpoint.URL); err != nil {
			return fmt.Errorf("invalid URL %s: %w", endpoint.URL, err)
		}
	}
	return nil
}

var serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9@:._-]+$`)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

var httpsClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

func checkServices(services []Service, messages *[]string) bool {
	var errCount int
	for _, service := range services {
		if !serviceNameRegex.MatchString(service.Name) {
			addToOutputMessages(messages, "Service Name: %s is invalid", service.Name)
			errCount++
			continue
		}
		cmd := exec.Command("systemctl", "is-active", service.Name)
		output, err := cmd.Output()
		status := strings.TrimSpace(string(output))
		if err != nil || status != service.Status {
			addToOutputMessages(messages, "Service Name: %s, Expected Status: %s, Actual Status: %s", service.Name, service.Status, status)
			errCount++
		} else {
			addToOutputMessages(messages, "Service Name: %s, Status: %s is as expected", service.Name, service.Status)
		}
	}
	return errCount == 0
}

func checkPorts(ports []Port, messages *[]string) bool {
	var errCount int
	for _, port := range ports {
		address := net.JoinHostPort(port.Address, strconv.Itoa(port.Port))
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err != nil {
			addToOutputMessages(messages, "Port Name: %s, Port: %d is not available", port.Name, port.Port)
			errCount++
		} else {
			addToOutputMessages(messages, "Port Name: %s, Port: %d is available", port.Name, port.Port)
			conn.Close()
		}
	}
	return errCount == 0
}

func checkEndpoints(endpoints []Endpoint, messages *[]string) bool {
	var errCount int
	for _, endpoint := range endpoints {
		var resp *http.Response
		var err error

		if strings.HasPrefix(endpoint.URL, "https://") {
			resp, err = httpsClient.Get(endpoint.URL)
		} else {
			resp, err = httpClient.Get(endpoint.URL)
		}

		if err != nil {
			addToOutputMessages(messages, "Endpoint Name: %s, URL: %s is not reachable", endpoint.Name, endpoint.URL)
			errCount++
			continue
		}

		statuses := append(endpoint.Statuses, endpoint.Status)
		if contains(statuses, resp.StatusCode) {
			addToOutputMessages(messages, "Endpoint Name: %s, URL: %s, Status: %d is as expected", endpoint.Name, endpoint.URL, resp.StatusCode)
		} else {
			addToOutputMessages(messages, "Endpoint Name: %s, URL: %s, Status: %d is not as expected, got: %d", endpoint.Name, endpoint.URL, endpoint.Status, resp.StatusCode)
			errCount++
		}

		resp.Body.Close() // Close immediately instead of defer to prevent resource leak
	}
	return errCount == 0
}

func addToOutputMessages(messages *[]string, format string, a ...interface{}) {
	message := fmt.Sprintf(format, a...)
	*messages = append(*messages, message)
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
