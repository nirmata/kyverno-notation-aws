ARG BUILDER_IMAGE="golang:1.24.4-alpine3.22"

FROM --platform=$BUILDPLATFORM $BUILDER_IMAGE AS builder

WORKDIR /

COPY go.mod go.sum .
RUN go mod download

COPY . ./

ARG TARGETOS
ARG TARGETARCH
# Build Signer plugin from source — the pre-built CDN binary bundles an outdated
# AWS SDK that rejects the EKS Pod Identity endpoint (169.254.170.23).
RUN apk add --no-cache git && \
    git clone --depth 1 https://github.com/aws/aws-signer-notation-plugin.git /aws-signer-plugin && \
    cd /aws-signer-plugin && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -ldflags="-w -s" \
      -o /notation-com.amazonaws.signer.notation.plugin ./cmd

# Build Go binary
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o kyverno-notation-aws .

FROM gcr.io/distroless/static:nonroot
WORKDIR /

# Notation home
ENV PLUGINS_DIR=/plugins

COPY --from=builder notation-com.amazonaws.signer.notation.plugin plugins/com.amazonaws.signer.notation.plugin/notation-com.amazonaws.signer.notation.plugin

COPY --from=builder kyverno-notation-aws kyverno-notation-aws
ENTRYPOINT ["/kyverno-notation-aws"]
