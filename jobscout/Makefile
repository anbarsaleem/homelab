.PHONY: build clean run run-now tidy

BIN_DIR := bin
BINARY  := jobscout-linux-amd64

build: tidy
	GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/$(BINARY) .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)

run:
	go run . --config config.yml

run-now:
	go run . --config config.yml --run-now
