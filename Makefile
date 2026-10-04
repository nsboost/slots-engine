.PHONY: build test vet fmt run simulate play clean

build:
	go build ./...

test:
	go vet ./...
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run:
	go run ./cmd/server

simulate:
	go run ./cmd/simulate -spins=5000000

play:
	go run ./cmd/play -n=10

clean:
	rm -f /tmp/server
	go clean
