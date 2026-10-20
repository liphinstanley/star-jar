# syntax=docker/dockerfile:1

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
COPY --from=build /out/star-jar /app/star-jar
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV PORT=8080 DB_PATH=/data/star-jar.db TZ=Europe/Berlin
EXPOSE 8080
ENTRYPOINT ["/app/star-jar"]
