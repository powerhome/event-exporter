ARG GIT_REPO=github.com/powerhome/event-exporter
ARG BIN=event-exporter

FROM golang:1.11.4-alpine3.8
ARG GIT_REPO
ARG BIN
ENTRYPOINT ["/$BIN"]
RUN apk add --no-cache make git gcc musl-dev
COPY ./ /go/src/${GIT_REPO}
RUN cd /go/src/${GIT_REPO} &&\
    go get &&\
    make build BIN=${BIN}

FROM alpine:3.8
ARG GIT_REPO
ARG BIN
COPY --from=0 /go/src/${GIT_REPO}/bin/$BIN /
CMD ["-v", "4"]
