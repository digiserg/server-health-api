# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=server-health-api

# Build the project
build:
	$(GOBUILD) -o bin/$(BINARY_NAME) -v

# Clean the project
clean:
	$(GOCLEAN)
	rm -f bin/$(BINARY_NAME)

# Run the application
run: build
	./bin/$(BINARY_NAME)

.PHONY: build clean test deps run
