# GoReleaser builds the binary and then this image, once per platform. The
# binary lies under the platform directory that buildx names.
FROM gcr.io/distroless/static:nonroot

# The image holds no CSS and no JavaScript of its own. Every static file is
# compiled into the binary.
EXPOSE 8080

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/chat /chat

# 65532 is the nonroot user of the base image.
USER 65532:65532

ENTRYPOINT ["/chat"]
