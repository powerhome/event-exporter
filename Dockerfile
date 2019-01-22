FROM golang:1.9.2-alpine3.7

ENV PKG=$GOPATH/github.com/bcdonadio/event-exporter
RUN apk add --no-cache make git
ADD . $PKG/
RUN echo $PKG &&\
    cd $PKG &&\
    go get &&\
    make build

FROM alpine:3.7
COPY --from=0 $GOBIN/event-exporter /
CMD ["/event-exporter", "-v", "4"]
