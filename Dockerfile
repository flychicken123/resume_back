# Build stage
FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create app directory
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Copy static files and templates
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/generate_resume.py ./generate_resume.py
COPY --from=builder /app/config.cfg ./config.cfg

# Expose port 8081
EXPOSE 8081

# Run the application
CMD ["./main"]
