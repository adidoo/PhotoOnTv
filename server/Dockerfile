FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /photoontv .

FROM alpine:3.20
# vips (+ libheif pour les HEIC) et ffmpeg génèrent les vignettes absentes ; ionice (util-linux-misc)
# met le balayage de fond en priorité disque minimale
RUN apk add --no-cache tzdata vips-tools vips-heif ffmpeg util-linux-misc
COPY --from=build /photoontv /usr/local/bin/photoontv
USER 65534:65534
ENV PHOTOS_ROOT=/photos LISTEN=:8080 CACHE_DIR=/cache
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/photoontv"]
