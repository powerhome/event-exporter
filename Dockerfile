FROM golang:1.11.4-alpine3.8
ENV REPO=github.com/bcdonadio/event-exporter
ENTRYPOINT ["/event-exporter"]
RUN apk add --no-cache make git
ADD ./* /go/src/$REPO/
RUN cd /go/src/$REPO/ &&\
    go get &&\
    make build

FROM alpine:3.8
COPY --from=0 /go/src/$REPO/bin/event-exporter /
CMD ["-v", "4"]
