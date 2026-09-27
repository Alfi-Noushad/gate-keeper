# Step 1: Build stage using Go official image
FROM golang:alpine AS builder

WORKDIR /app

# Copy dependency definitions and download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source files and build the Go binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gatekeeper .

# Step 2: Minimal runtime image
FROM alpine:latest

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/gatekeeper .

EXPOSE 8080

CMD ["./gatekeeper"]