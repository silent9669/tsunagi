.PHONY: build test e2e e2e-ui install
build:
	go build -o tsunagi .
test:
	go vet ./... && go test -race ./...
e2e: build
	./e2e/run.sh
e2e-ui:
	./e2e/ui.sh
install: build
	install -m 0755 tsunagi /usr/local/bin/tsunagi
