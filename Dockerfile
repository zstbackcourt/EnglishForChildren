# Default build on the wechat branch: native mini-program API for CloudRun.
FROM golang:1.27.3-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/wechat-server ./cmd/wechat-server
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wechat-server ./cmd/wechat-server

# Pinned public teaching assets, independent of local credentials or save files.
FROM alpine:3.23 AS materials
RUN apk add --no-cache git ca-certificates
WORKDIR /source/course
RUN git init . && git remote add origin https://github.com/zstbackcourt/New-Concept-English.git \
 && git sparse-checkout set --no-cone '/新概念英语第1册美音（MP3+LRC）/' \
 && git fetch --depth=1 --filter=blob:none origin 635746ac5f696b193311bc1a7e8ffac3b252eb75 \
 && git checkout --detach FETCH_HEAD
WORKDIR /source/art
RUN git init . && git remote add origin https://github.com/zstbackcourt/EnglishForChildren.git \
 && git sparse-checkout set web/assets/scenes web/assets/pages \
 && git fetch --depth=1 --filter=blob:none origin 7a4c9a63cca2471ec8b7157e0b36cf2bc1cacbef \
 && git checkout --detach FETCH_HEAD

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S game && adduser -S -G game game
WORKDIR /app
COPY --from=build /out/wechat-server /usr/local/bin/wechat-server
COPY --from=materials ["/source/course/新概念英语第1册美音（MP3+LRC）", "/app/New-Concept-English/新概念英语第1册美音（MP3+LRC）"]
COPY --from=materials /source/art/web/assets/scenes ./web/assets/scenes
COPY --from=materials /source/art/web/assets/pages ./web/assets/pages
COPY content ./content
COPY deploy/wechat/release-id.txt ./deploy/wechat/release-id.txt
COPY --chmod=755 deploy/wechat/cloudrun-entrypoint.sh /usr/local/bin/cloudrun-entrypoint
USER game
ENV PORT=8080 WECHAT_STORAGE=mysql
EXPOSE 8080
ENTRYPOINT ["cloudrun-entrypoint"]
