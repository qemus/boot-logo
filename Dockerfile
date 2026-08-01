# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS builder

COPY src/go.mod src/go.sum /src/boot-logo/

WORKDIR /src/boot-logo

RUN go mod download

COPY tests/ /src/tests/
COPY src/ /src/boot-logo/

RUN go mod tidy \
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
