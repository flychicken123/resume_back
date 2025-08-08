# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application (only main.go to avoid conflicts)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main main.go

# Final stage
FROM debian:bookworm-slim

# Install wkhtmltopdf and fonts (Debian provides more reliable package)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       wkhtmltopdf \
       fontconfig \
       fonts-dejavu \
       python3 \
    && rm -rf /var/lib/apt/lists/*

# Create app directory and required subdirectories
WORKDIR /root/
RUN mkdir -p static templates

# Copy the binary and python script
COPY --from=builder /app/main .
COPY --from=builder /app/generate_resume.py .

# Expose port 8081
EXPOSE 8081

# Run the application
CMD ["./main"]