# Runtime-only Dockerfile for use with goreleaser
# Goreleaser pre-builds the binary and sends it to the docker context
FROM gcr.io/distroless/static:nonroot

# Copy the pre-built binary from goreleaser
COPY cub-scout /cub-scout

USER nonroot:nonroot

# Set HOME for kubeconfig
ENV HOME=/home/nonroot

ENTRYPOINT ["/cub-scout"]
CMD ["version"]
