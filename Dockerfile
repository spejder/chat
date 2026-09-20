# GoReleaser builds the binary and then this image, once per platform. The
# binary lies under the platform directory that buildx names.
#
# The image holds the binary and nothing else. Every static file lives inside
# the binary, and the root certificates come from the fallback package that
# cmd/chat imports.
FROM scratch

EXPOSE 8080

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/chat /chat

# A number, because an image from scratch has no list of users.
USER 65532:65532

ENTRYPOINT ["/chat"]
