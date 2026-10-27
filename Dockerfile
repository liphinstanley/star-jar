# syntax=docker/dockerfile:1

# Litestream: backs the SQLite file up to Cloud Storage and restores it on start
FROM litestream/litestream:0.5.17 AS litestream

# Build stage: compile on the machine's own CPU, cross-compile for the target (Cloud Run is amd64)
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/star-jar . \
 && mkdir /out/data

# Run stage: no shell, no package manager, runs as a non-root user
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY litestream.yml /etc/litestream.yml
COPY --from=build /out/star-jar /app/star-jar
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV PORT=8080 DB_PATH=/data/star-jar.db TZ=Europe/Berlin
EXPOSE 8080
# restore the database if it's missing, start the app, replicate every change; needs REPLICA_URL
ENTRYPOINT ["/usr/local/bin/litestream", "replicate", "-restore-if-db-not-exists", "-exec", "/app/star-jar"]
