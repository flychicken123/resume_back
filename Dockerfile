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

# Install basic dependencies and wkhtmltopdf
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       fontconfig \
       fonts-dejavu \
       wkhtmltopdf \
       python3 \
       python3-pip \
    && ln -s /usr/bin/python3 /usr/bin/python \
    && rm -rf /var/lib/apt/lists/*

# Python libs for parsing
RUN pip3 install --no-cache-dir pdfminer.six python-docx

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