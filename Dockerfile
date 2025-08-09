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
# Remove expensive flags to reduce memory usage during build
RUN CGO_ENABLED=0 GOOS=linux go build -o main main.go

# Final stage
FROM debian:bookworm-slim

# Install wkhtmltopdf 0.12.6 to match local version and fonts for consistent rendering
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       fontconfig \
       fonts-dejavu \
       fonts-liberation \
       fonts-noto \
       fonts-noto-cjk \
       wget \
       gnupg \
    && wget -qO- https://github.com/wkhtmltopdf/packaging/releases/download/0.12.6-1/wkhtmltox_0.12.6-1.bullseye_amd64.deb \
    && apt-get install -y ./wkhtmltox_0.12.6-1.bullseye_amd64.deb \
    && rm wkhtmltox_0.12.6-1.bullseye_amd64.deb \
    && apt-get install -y --no-install-recommends \
       python3 \
       python3-pip \
       python3-pdfminer \
       python3-docx \
    && ln -s /usr/bin/python3 /usr/bin/python \
    && rm -rf /var/lib/apt/lists/*

# Speed up pip and reduce memory/disk usage
ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1

# Python libs via Debian packages (lighter than pip on low-memory builders)
# (No additional pip installs required)

# Create app directory and required subdirectories
WORKDIR /root/
RUN mkdir -p static templates uploads

# Copy the binary and scripts
COPY --from=builder /app/main .
COPY --from=builder /app/generate_resume.py .
COPY --from=builder /app/parse_resume.py .

# Expose port 8081
EXPOSE 8081

# Run the application
CMD ["./main"]