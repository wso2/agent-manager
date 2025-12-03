# Build stage
FROM golang:1.25-alpine AS build-env

# Create app directory
RUN mkdir /app
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/traces-observer -buildvcs=false

# Runtime stage
FROM alpine:3

# Copy the binary from builder
COPY --from=build-env /go/bin/traces-observer /go/bin/traces-observer

# Expose the application port
EXPOSE 9098

ENTRYPOINT ["/go/bin/traces-observer"]


