ARG BUILDER_IMAGE="golang:1.26.8-alpine3.24"

FROM --platform=$BUILDPLATFORM $BUILDER_IMAGE AS builder

WORKDIR /

COPY go.mod go.sum .
RUN go mod download

COPY . ./

ARG TARGETOS
ARG TARGETARCH
# Build the signer plugin from source so it links against the current Go
# toolchain. The prebuilt CDN binary ships compiled with Go 1.23.6 and carries
# that stdlib's CVEs, which no change in this repo's go.mod can reach.
# The pseudo-version pins upstream main at 93a2aa12f47c (2025-12-16) and is
# immutable, unlike a branch ref. Its checksum is verified against
# sum.golang.org, so a compromised upstream cannot silently enter a release.
ARG SIGNER_PLUGIN_PKG="github.com/aws/aws-signer-notation-plugin/cmd"
ARG SIGNER_PLUGIN_VERSION="v1.0.2293-0.20251216222753-93a2aa12f47c"
RUN mkdir /aws-signer-plugin && cd /aws-signer-plugin && \
    go mod init plugin-pin && \
    go get ${SIGNER_PLUGIN_PKG}@${SIGNER_PLUGIN_VERSION} && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -ldflags="-w -s" \
      -o /notation-com.amazonaws.signer.notation.plugin ${SIGNER_PLUGIN_PKG}

# Build Go binary
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o kyverno-notation-aws .

FROM gcr.io/distroless/static:nonroot
WORKDIR /

# Notation home
ENV PLUGINS_DIR=/plugins

COPY --from=builder /notation-com.amazonaws.signer.notation.plugin plugins/com.amazonaws.signer.notation.plugin/notation-com.amazonaws.signer.notation.plugin

COPY --from=builder kyverno-notation-aws kyverno-notation-aws
ENTRYPOINT ["/kyverno-notation-aws"]
