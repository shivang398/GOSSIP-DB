# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o gossipdb cmd/server/main.go

# Run stage
FROM alpine:latest

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/gossipdb .

# Expose port
EXPOSE 8080

# Run the binary
CMD ["./gossipdb"]
