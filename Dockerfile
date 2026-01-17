# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /cub-scout ./cmd/cub-scout

# Runtime stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /cub-scout /cub-scout

USER nonroot:nonroot

ENTRYPOINT ["/cub-scout"]
CMD ["version"]
