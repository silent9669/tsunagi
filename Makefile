.PHONY: build test e2e install
build:
	go build -o tsunagi .
test:
	go vet ./... && go test -race ./...
e2e: build
	./e2e/run.sh
install: build
	install -m 0755 tsunagi /usr/local/bin/tsunagi
