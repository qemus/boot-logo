# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS builder

RUN apk --no-cache add \
    ca-certificates \
    unzip \
    wget

ARG FIANO_VERSION="1.2.0"

RUN wget -q \
      "https://github.com/qemus/fiano/releases/download/v${FIANO_VERSION}/module.zip" \
      -O /tmp/fiano.zip \
    && unzip -q /tmp/fiano.zip -d /src \
    && rm /tmp/fiano.zip \
    && test -f /src/fiano/go.mod \
    && grep -Fxq \
      "module github.com/linuxboot/fiano" \
      /src/fiano/go.mod

COPY src/ /src/boot-logo/

WORKDIR /src/boot-logo

RUN go mod edit \
      -replace=github.com/linuxboot/fiano=/src/fiano \
    && go mod tidy \
    && go test ./...

ARG VERSION_ARG="0.0"
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 \
    GOOS="$TARGETOS" \
    GOARCH="$TARGETARCH" \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION_ARG}" \
      -o /boot-logo.bin \
      .

FROM scratch

COPY --chmod=755 --from=builder /boot-logo.bin /boot-logo.bin

ENTRYPOINT ["/boot-logo.bin"]
