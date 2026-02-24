BINARY   = forcedesk-agent.exe
GOFLAGS  = -mod=mod
LDFLAGS  = -s -w

.PHONY: build build-local build-debug run-debug resource install-tools tidy clean

## install-tools: install build-time tools (winres)
install-tools:
	go install github.com/tc-hib/go-winres@latest

## resource: generate rsrc_windows_amd64.syso (embeds icon + version info into the .exe)
resource:
	$(shell go env GOPATH)/bin/go-winres make --arch amd64

## build: compile the production Windows binary (stripped, with embedded icon)
build: resource
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY) .

## build-local: compile a native binary for the current OS (macOS/Linux, for testing)
build-local:
	go build $(GOFLAGS) -o forcedesk-agent .

## build-debug: compile without stripping (keeps symbols for debugging)
build-debug: resource
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o $(BINARY) .

## run-debug: run the scheduler locally on the current OS (logs to stdout)
run-debug:
	go run $(GOFLAGS) . debug

## tidy: update go.sum and prune unused dependencies
tidy:
	go mod tidy

## clean: remove compiled binaries and generated resources
clean:
	rm -f $(BINARY) forcedesk-agent rsrc_windows_amd64.syso
