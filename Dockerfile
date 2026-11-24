FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends libsqlite3-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod *.go ./
RUN GOMAXPROCS=1 go test -p=1 -trimpath ./... && \
    GOMAXPROCS=1 go build -p=1 -trimpath -ldflags="-s -w" -o /seed-panel .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends libsqlite3-0 && rm -rf /var/lib/apt/lists/*
COPY --from=build /seed-panel /usr/local/bin/seed-panel
COPY web /usr/share/seed-panel
ENTRYPOINT ["/usr/local/bin/seed-panel"]
