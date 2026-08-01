# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

COPY src/ /src/boot-logo/
WORKDIR /src/boot-logo

RUN go mod download

ARG VERSION_ARG="0.0"
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -installsuffix cgo -ldflags "-X main.Version=$VERSION_ARG" -o /src/boot-logo/main .

FROM scratch

COPY --chmod=755 --from=builder /src/boot-logo/main /boot-logo.bin

ENTRYPOINT ["/boot-logo.bin"]
