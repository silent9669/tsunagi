.PHONY: build test install
build:
	go build -o tsunagi .
test:
	go vet ./... && go test -race ./...
install: build
	install -m 0755 tsunagi /usr/local/bin/tsunagi
