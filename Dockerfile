# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Limit Go parallelism for memory efficiency
ENV GOMAXPROCS=1
RUN GOMAXPROCS=1 CGO_ENABLED=0 GOOS=linux go build -o main main.go

# Final stage (Ubuntu 20.04 for wkhtmltopdf 0.12.6-1 with patched Qt)
FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive
ENV WKHTML_VERSION=0.12.6-1

# Install system dependencies and fonts
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates curl fontconfig xz-utils \
    libjpeg-turbo8 libpng16-16 libxrender1 libxtst6 libssl1.1 \
    fonts-dejavu fonts-liberation fonts-noto fonts-noto-cjk \
    python3 python3-pip poppler-utils tesseract-ocr tesseract-ocr-eng gdebi-core && \
    rm -rf /var/lib/apt/lists/*

# Install wkhtmltopdf 0.12.6-1 with patched Qt
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
    gdebi -n /tmp/wkhtmltox.deb; \
    rm -f /tmp/wkhtmltox.deb

# Install Python packages
RUN pip3 install --no-cache-dir python-docx pymupdf pdfminer.six && \
    ln -sf /usr/bin/python3 /usr/bin/python && \
    wkhtmltopdf --version && \
    fc-cache -f -v

WORKDIR /root/
RUN mkdir -p static templates uploads
COPY --from=builder /app/main .
COPY --from=builder /app/generate_resume.py .
COPY --from=builder /app/parse_resume.py .
EXPOSE 8081
CMD ["./main"]