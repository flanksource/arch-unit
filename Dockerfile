# Multi-stage build: Java build -> Go build -> Runtime
FROM openjdk:17-jdk-slim AS java-builder

WORKDIR /app
COPY analysis/java/ analysis/java/

# Build the Java AST extractor
WORKDIR /app/analysis/java
RUN apt-get update && apt-get install -y curl unzip && \
    mkdir -p gradle/wrapper && \
    curl -L https://services.gradle.org/distributions/gradle-8.5-bin.zip -o gradle-8.5-bin.zip && \
    unzip -q gradle-8.5-bin.zip && \
    ./gradle-8.5/bin/gradle wrapper --gradle-version 8.5 && \
    rm -rf gradle-8.5 gradle-8.5-bin.zip && \
    ./gradlew clean shadowJar

# Go build stage
FROM golang:1.25-alpine AS go-builder

WORKDIR /app

# Copy Go modules and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy the Java JAR from the previous stage
COPY --from=java-builder /app/analysis/java/java_ast_extractor.jar analysis/java/

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o arch-unit .

# Runtime stage
FROM openjdk:17-jre-slim

WORKDIR /app

# Install necessary runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    git \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary from the go-builder stage
COPY --from=go-builder /app/arch-unit /usr/local/bin/

# Create a non-root user
RUN useradd -r -s /bin/false archunit

USER archunit

ENTRYPOINT ["arch-unit"]