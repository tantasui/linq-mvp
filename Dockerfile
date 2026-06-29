FROM golang:1.23-bookworm AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /app/server ./cmd/server/ && ls -lh /app/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /app/server /app/server
COPY --from=build /app/migrations /app/migrations
RUN ls -lh /app/server
EXPOSE 8080
ENTRYPOINT ["/bin/sh", "-c", "/app/server 2>&1; echo \"--- SERVER EXITED code=$? ---\"; sleep 3600"]
