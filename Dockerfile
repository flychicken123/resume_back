# Build stage (Debian-based to match final image toolchain)
FROM golang:1.24 AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Copy vendor directory
COPY vendor ./vendor

# Build the application using vendored packages
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o main main.go

# Final stage (Ubuntu focal to support wkhtmltopdf 0.12.6-1 .deb with patched Qt)
FROM ubuntu:20.04

# Install essential system packages and fonts
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates curl xz-utils fontconfig \
    fonts-dejavu fonts-liberation fonts-noto fonts-noto-cjk \
    python3 python3-pip ghostscript && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install wkhtmltopdf and Python libraries
ENV WKHTML_VERSION=0.12.6-1
RUN set -eux; \
  arch="$(uname -m)"; \
  case "$arch" in \
    x86_64)  WK_ARCH=amd64 ;; \
    aarch64) WK_ARCH=arm64 ;; \
    *) echo "Unsupported arch: $arch"; exit 1 ;; \
  esac; \
  DEB_URL="https://github.com/wkhtmltopdf/packaging/releases/download/${WKHTML_VERSION}/wkhtmltox_${WKHTML_VERSION}.focal_${WK_ARCH}.deb"; \
  echo "Downloading $DEB_URL"; \
  curl -fSL --retry 5 --retry-connrefused -o /tmp/wkhtmltox.deb "$DEB_URL"; \
  dpkg -i /tmp/wkhtmltox.deb || apt-get -f install -y; \
  rm -f /tmp/wkhtmltox.deb; \
  pip3 install --no-cache-dir python-docx pymupdf; \
  ln -sf /usr/bin/python3 /usr/bin/python; \
  wkhtmltopdf --version; \
  fc-cache -f -v

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