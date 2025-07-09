ARG BUILDER_IMAGE="golang:1.24.4-alpine3.22"

FROM --platform=$BUILDPLATFORM $BUILDER_IMAGE AS builder

WORKDIR /

COPY go.mod go.sum .
RUN go mod download

COPY . ./

ARG TARGETOS
ARG TARGETARCH
# Get Signer plugin binary
ARG SIGNER_BINARY_LINK="https://d2hvyiie56hcat.cloudfront.net/linux/amd64/plugin/latest/notation-aws-signer-plugin.zip"
ARG SIGNER_BINARY_FILE="notation-aws-signer-plugin.zip"
RUN wget -O ${SIGNER_BINARY_FILE} ${SIGNER_BINARY_LINK} 
RUN apk update && \
    apk add unzip && \
    unzip -o ${SIGNER_BINARY_FILE}

# Build Go binary
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o kyverno-notation-aws .

FROM gcr.io/distroless/static:nonroot
WORKDIR /

# Notation home
ENV PLUGINS_DIR=/plugins

COPY --from=builder notation-com.amazonaws.signer.notation.plugin plugins/com.amazonaws.signer.notation.plugin/notation-com.amazonaws.signer.notation.plugin

COPY --from=builder kyverno-notation-aws kyverno-notation-aws
ENTRYPOINT ["/kyverno-notation-aws"]
