# Stage 1: Build the Go binary
FROM golang:1.23.10-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main ./cmd/main.go

# Stage 2: Create a minimal image
FROM alpine:3.18 AS runtime
WORKDIR /app
RUN apk --no-cache add ca-certificates curl tzdata
COPY --from=builder /app/main /app/main
RUN chmod +x /app/main
CMD ["/app/main"]
