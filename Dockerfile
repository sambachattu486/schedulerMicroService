FROM golang:alpine AS builder

# Set necessary environment variables needed for our image
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Move to working directory /build
WORKDIR /build

# Copy and download dependencies using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build -o main .

# Move to /dist directory as the place for resulting binary folder
WORKDIR /dist

# Copy binary from build to main folder
RUN cp /build/main .
RUN cp /build/.config .

# Build a small image
FROM alpine

# Install PostgreSQL client library
# RUN apk update && apk add --no-cache postgresql-client

WORKDIR /app

COPY --from=builder /dist/main /app/
COPY --from=builder /dist/.config /app/

EXPOSE 6003

# Command to run
ENTRYPOINT ["/app/main"]



