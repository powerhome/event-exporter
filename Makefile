
GOFILES_NOVENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*")
UNITTEST_PACKAGES = $(shell go list ./... | grep -v /vendor/ | grep -v integration_test)
IMG_REPO ?= quay.io/powerhome/event-exporter
IMG_TAG ?= v0.2.0
BIN ?= event-exporter

all: fmt vet build

fmt:
	gofmt -l -w ${GOFILES_NOVENDOR}

vet:
	go vet ${UNITTEST_PACKAGES}

build:
	GO111MODULE=on go build -ldflags -s -v -o bin/${BIN} .

run: build
	bin/${BIN}

test:
	go test -ldflags -s -v --cover ${UNITTEST_PACKAGES}

image:
	docker build -t ${IMG_REPO}:${IMG_TAG} .

push:
	docker push ${IMG_REPO}:${IMG_TAG}

docker: image push
