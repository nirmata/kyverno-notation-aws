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
ARG SIGNER_PLUGIN_REF="main"
RUN apk add --no-cache git && \
    git clone --depth 1 --branch ${SIGNER_PLUGIN_REF} \
      https://github.com/aws/aws-signer-notation-plugin.git /aws-signer-plugin && \
    cd /aws-signer-plugin && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -ldflags="-w -s" \
      -o /notation-com.amazonaws.signer.notation.plugin ./cmd

# Build Go binary
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o kyverno-notation-aws .

FROM gcr.io/distroless/static:nonroot
WORKDIR /

# Notation home
ENV PLUGINS_DIR=/plugins

COPY --from=builder /notation-com.amazonaws.signer.notation.plugin plugins/com.amazonaws.signer.notation.plugin/notation-com.amazonaws.signer.notation.plugin

COPY --from=builder kyverno-notation-aws kyverno-notation-aws
ENTRYPOINT ["/kyverno-notation-aws"]
