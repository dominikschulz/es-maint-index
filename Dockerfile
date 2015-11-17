FROM golang:1.5.1
MAINTAINER Dominik Schulz <dominik.schulz@gauner.org>

ENV GOPATH /go/src/github.com/dominikschulz/es-maint-index/Godeps/_workspace/:/go

ADD .   /go/src/github.com/dominikschulz/es-maint-index
WORKDIR /go/src/github.com/dominikschulz/es-maint-index

RUN go install

CMD [ "/go/bin/es-maint-index" ]
