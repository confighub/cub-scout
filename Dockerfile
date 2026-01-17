# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build both binaries
ARG BUILD_TAG=dev
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X main.BuildTag=${BUILD_TAG} -X main.BuildDate=${BUILD_DATE}" -o /cub-agent ./cmd/cub-agent
RUN CGO_ENABLED=0 GOOS=linux go build -o /agent ./cmd/agent

# Runtime stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /cub-agent /cub-agent
COPY --from=builder /agent /agent

USER nonroot:nonroot

# Default to running the agent daemon
ENTRYPOINT ["/cub-agent"]
CMD ["agent", "run"]
