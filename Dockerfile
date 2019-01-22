FROM golang:1.11.4-alpine3.8
ENV GOPATH=/go \
    PKG=$GOPATH/src/github.com/bcdonadio/event-exporter
RUN apk add --no-cache make git
ADD . $PKG/
RUN echo $PKG &&\
    cd $PKG &&\
    go get &&\
    make build

FROM alpine:3.8
COPY --from=0 /go/bin/event-exporter /
CMD ["/event-exporter", "-v", "4"]
