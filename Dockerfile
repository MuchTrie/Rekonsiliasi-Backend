# ---- Build stage ----
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build binary
RUN go build -o server ./cmd/server/main.go

# ---- Runtime stage ----
FROM alpine:latest

WORKDIR /app

# Copy only built binary
COPY --from=builder /app/server ./server

EXPOSE 8080

CMD ["/app/server"]    