FROM golang:1.6-alpine
MAINTAINER Dominik Schulz <dominik.schulz@gauner.org>

RUN apk --update add \
  ca-certificates \
  curl \
  git \
  make \
  && rm -rf /var/cache/apk/*

ENV GOPATH /go
ENV GOBIN $GOPATH/bin
ENV PATH $GOBIN:$PATH

RUN mkdir -p "$GOPATH/src" "$GOBIN" && chmod -R 777 "$GOPATH"

ADD .   /go/src/github.com/dominikschulz/es-maint-index
WORKDIR /go/src/github.com/dominikschulz/es-maint-index

RUN make install

CMD [ "/go/bin/es-maint-index" ]
