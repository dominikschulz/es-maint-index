# get the current git tag
TAG=$(shell git rev-parse --short=8 HEAD)
NAME=es-maint-index
VERSION?=git

BUILD_TIME=`date +%FT%T%z`
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${TAG}"
SOURCEDIR=.
SOURCES := $(shell find $(SOURCEDIR) -name '*.go')

.DEFAULT_GOAL: all
.PHONY: test install clean all

all: maint

maint: $(SOURCES)
	go build -a ${LDFLAGS}
	strip es-maint-index

install:
	go install ${LDFLAGS}

clean:
	if [ -f es-maint-index ] ; then rm es-maint-index ; fi

test: test-base

test-base:
	go test -v -race ./...

docker-test: docker-test-base

docker-test-base: docker-image-tag
	docker run ${NAME}:${TAG} make test-base

docker-test-meta: docker-image-tag
	docker run ${NAME}:${TAG} make test-meta

docker-test-coverage: docker-image-tag
	docker run ${NAME}:${TAG} make test-coverage

docker-image: docker-image-tag docker-image-latest

docker-image-tag:
	docker build -t ${NAME}:${TAG} .
