.PHONY: run build test docker-build docker-run clean

BINARY_NAME=gossipdb
BUILD_DIR=bin

run:
	go run cmd/server/main.go

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) cmd/server/main.go

test:
	go test -v ./...

docker-build:
	docker build -t $(BINARY_NAME):latest .

docker-run:
	docker run -p 8080:8080 --env PORT=8080 $(BINARY_NAME):latest

clean:
	rm -rf $(BUILD_DIR)
	go clean
