FROM golang:1.27.3-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/game ./cmd/server

FROM alpine:3.23 AS materials
RUN apk add --no-cache git
WORKDIR /source
RUN git init . && git remote add origin https://github.com/zstbackcourt/New-Concept-English.git \
 && git sparse-checkout set --no-cone '/新概念英语第1册美音（MP3+LRC）/' \
 && git fetch --depth=1 --filter=blob:none origin 635746ac5f696b193311bc1a7e8ffac3b252eb75 \
 && git checkout --detach FETCH_HEAD

FROM alpine:3.23
WORKDIR /app
RUN apk add --no-cache su-exec && addgroup -S game && adduser -S -G game game && mkdir data && chown game:game data
COPY --from=build /out/game /usr/local/bin/game
COPY --from=materials ["/source/新概念英语第1册美音（MP3+LRC）", "/app/New-Concept-English/新概念英语第1册美音（MP3+LRC）"]
COPY content ./content
COPY web ./web
COPY --chmod=755 scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["docker-entrypoint"]
