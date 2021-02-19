SHELL := /bin/bash # Use bash syntax

# Set up variables
GO111MODULE=on

ELASTICACHE_API_PATH="$(shell echo $(shell go env GOPATH))/pkg/mod/github.com/aws/aws-sdk-go@v1.37.4/service/elasticache/elasticacheiface"
SERVICE_CONTROLLER_SRC_PATH="$(shell pwd)"

# Build ldflags
VERSION ?= "v0.0.0"
GITCOMMIT=$(shell git rev-parse HEAD)
BUILDDATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GO_LDFLAGS=-ldflags "-X main.version=$(VERSION) \
			-X main.buildHash=$(GITCOMMIT) \
			-X main.buildDate=$(BUILDDATE)"

.PHONY: all test local-test clean-mocks mocks

all: test

test: | mocks				## Run code tests
	go test -v ./...

local-test: | mocks		## Run code tests using go.local.mod file
	go test -modfile=go.local.mod -v ./...

clean-mocks:	## Remove mocks directory
	rm -rf mocks

install-mockery:
	@scripts/install-mockery.sh

mocks: install-mockery ## Build mocks
	go get -d github.com/aws/aws-sdk-go@v1.37.4
	@echo "building mocks for $(ELASTICACHE_API_PATH) ... "
	@pushd $(ELASTICACHE_API_PATH) 1>/dev/null; \
	$(SERVICE_CONTROLLER_SRC_PATH)/bin/mockery --all --dir=. --output=$(SERVICE_CONTROLLER_SRC_PATH)/mocks/aws-sdk-go/elasticache/ ; \
	popd 1>/dev/null;
	@echo "ok."

help:           	## Show this help.
	@grep -F -h "##" $(MAKEFILE_LIST) | grep -F -v grep | sed -e 's/\\$$//' \
		| awk -F'[:#]' '{print $$1 = sprintf("%-30s", $$1), $$4}'
