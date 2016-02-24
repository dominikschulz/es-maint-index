FROM golang:1.6
MAINTAINER Dominik Schulz <dominik.schulz@gauner.org>

ADD .   /go/src/github.com/dominikschulz/es-maint-index
WORKDIR /go/src/github.com/dominikschulz/es-maint-index

RUN go install

CMD [ "/go/bin/es-maint-index" ]
