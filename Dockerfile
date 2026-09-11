# Runtime-only Dockerfile for use with goreleaser
# Goreleaser pre-builds the binary and sends it to the docker context
FROM gcr.io/distroless/static:nonroot

# Copy the pre-built binary from goreleaser
COPY cub-scout /cub-scout

# A numeric UID lets Kubernetes validate runAsNonRoot without starting the container.
USER 65532:65532

# Set HOME for kubeconfig
ENV HOME=/home/nonroot

ENTRYPOINT ["/cub-scout"]
CMD ["version"]
