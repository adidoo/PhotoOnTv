FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /photoontv .

FROM alpine:3.20
RUN apk add --no-cache tzdata
COPY --from=build /photoontv /usr/local/bin/photoontv
USER 65534:65534
ENV PHOTOS_ROOT=/photos LISTEN=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/photoontv"]
