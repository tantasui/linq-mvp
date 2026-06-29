.PHONY: build run tidy smoke

build:
	GOWORK=off CGO_ENABLED=1 go build -o server ./cmd/server/

run:
	GOWORK=off CGO_ENABLED=1 go run ./cmd/server/

tidy:
	GOWORK=off go mod tidy

# Register a Sui address (set ADDRESS= on the command line)
# make register ADDRESS=0xYOUR_SUI_ADDRESS
register:
	curl -s -X POST http://localhost:8080/wallet/register \
	  -H "Content-Type: application/json" \
	  -d '{"address":"$(ADDRESS)","network_type":"sui"}' | jq .

# Check balance
# make balance ADDRESS=0xYOUR_SUI_ADDRESS
balance:
	curl -s http://localhost:8080/balance/$(ADDRESS) | jq .

health:
	curl -s http://localhost:8080/health | jq .
