FROM golang:1.8-alpine3.6 as builder

ADD . /go/src/github.com/dominikschulz/es-maint-index
WORKDIR /go/src/github.com/dominikschulz/es-maint-index

RUN go install

FROM alpine:3.6

COPY --from=builder /go/bin/es-maint-index /usr/local/bin/es-maint-index
CMD [ "/usr/local/bin/es-maint-index" ]
EXPOSE 8080
