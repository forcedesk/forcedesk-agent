BINARY   = forcedesk-agent.exe
GOFLAGS  = -mod=mod
LDFLAGS  = -s -w

.PHONY: build build-debug run-debug clean

## build: compile the production Windows binary (stripped, GUI subsystem)
build:
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY) .

## build-debug: compile without stripping (keeps symbols for debugging)
build-debug:
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o $(BINARY) .

## run-debug: run the scheduler locally on the current OS (logs to stdout)
run-debug:
	go run $(GOFLAGS) . debug

## tidy: update go.sum and prune unused dependencies
tidy:
	go mod tidy

## clean: remove compiled binaries
clean:
	rm -f $(BINARY)
