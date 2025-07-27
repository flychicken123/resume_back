FROM python:3.9-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements first to leverage Docker cache
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Install spaCy and download model
RUN pip install spacy==3.8.7 && \
    python -m spacy download en_core_web_sm

# Download NLTK data during build
RUN python -c "import nltk; \
    nltk.download('stopwords'); \
    nltk.download('punkt'); \
    nltk.download('averaged_perceptron_tagger'); \
    nltk.download('wordnet'); \
    nltk.download('maxent_ne_chunker'); \
    nltk.download('words'); \
    nltk.download('omw-1.4')"

# Create config directory and copy config file
RUN mkdir -p /usr/local/lib/python3.9/site-packages/pyresparser
COPY config.cfg /usr/local/lib/python3.9/site-packages/pyresparser/

# Copy the rest of the application
COPY . .

# Expose the port the app runs on
EXPOSE 5000

# Command to run the application
CMD ["python", "app.py"] 