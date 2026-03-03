# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Copy module files first for caching
COPY go_app/go.mod go_app/go.sum ./
RUN go mod download

# Copy source code
COPY go_app/ .

# Build binaries
RUN go build -o /bin/monitor ./cmd/monitor
RUN go build -o /bin/notifier ./cmd/notifier

# Final stage
FROM alpine:latest

WORKDIR /app

# Install certificates for HTTPS and bash for script
RUN apk --no-cache add ca-certificates bash

# Copy binaries from builder
COPY --from=builder /bin/monitor .
COPY --from=builder /bin/notifier .

# Create startup script to run both services
# Run monitor in background, notifier in foreground
RUN echo -e '#!/bin/bash\n\
    ./monitor &\n\
    ./notifier\n\
    ' > /app/start.sh && chmod +x /app/start.sh

CMD ["/app/start.sh"]
