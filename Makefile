# duet build entry points (goal 3.1). Artifacts go to release/, dev builds to .temp/build/.

VERSION ?= 0.1.0-dev
ROOT := $(shell pwd)

.PHONY: test test-coordinator test-node build app release clean

test: test-coordinator test-node

test-coordinator:
	cd coordinator && go test ./...

test-node:
	cd node-app && swift test

build:
	mkdir -p .temp/build
	cd coordinator && go build -ldflags "-X main.version=$(VERSION)" -o $(ROOT)/.temp/build/duet-coordinator ./cmd/duet-coordinator
	cd node-app && swift build -c release

app: build
	OUT_DIR=$(ROOT)/.temp/build VERSION=$(VERSION) scripts/build-app.sh

release: test
	rm -rf release && mkdir -p release
	cd coordinator && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $(ROOT)/release/duet-coordinator-$(VERSION)-linux-amd64 ./cmd/duet-coordinator
	cd node-app && swift build -c release
	OUT_DIR=$(ROOT)/release VERSION=$(VERSION) scripts/build-app.sh
	cd release && ditto -c -k --keepParent NodeApp.app NodeApp-$(VERSION).app.zip && rm -rf NodeApp.app
	cd release && shasum -a 256 * > checksums.txt
	@echo "release/ ready:" && ls -la release/

clean:
	rm -rf .temp/build release
	cd coordinator && go clean ./...
	cd node-app && swift package clean
