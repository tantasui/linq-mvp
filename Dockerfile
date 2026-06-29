FROM golang:1.22-bookworm AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o server ./cmd/server/

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /app/server .
COPY --from=build /app/migrations ./migrations
EXPOSE 8080
ENTRYPOINT ["./server"]
