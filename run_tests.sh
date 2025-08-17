#!/bin/bash

# Run all tests with coverage

echo "🧪 Running Backend Tests..."
echo "================================"

# Install test dependencies if needed
go get -u github.com/stretchr/testify/assert

# Run all tests with coverage
echo "📊 Running tests with coverage..."
go test -v -cover ./...

# Run tests with detailed coverage report
echo ""
echo "📈 Generating coverage report..."
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

echo ""
echo "✅ Tests completed!"
echo "📄 Coverage report saved to coverage.html"

# Run specific middleware tests
echo ""
echo "🔧 Running Middleware Tests..."
echo "--------------------------------"
go test -v ./middleware

# Show coverage percentage
echo ""
echo "📊 Coverage Summary:"
go test -cover ./... | grep coverage