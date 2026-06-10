# Build stages run natively on the build host ($BUILDPLATFORM) and
# cross-compile for the target — emulating Node/Go under QEMU for arm64 is
# orders of magnitude slower. Only the tiny runtime stage runs emulated.

# Stage 1: build web app (output is platform-independent static files)
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: cross-compile bastion binary for the target platform
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /app
COPY go.mod go.sum ./
COPY pkg/ pkg/
COPY bastion/ bastion/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w -X blackbox/pkg/version.Version=$VERSION" -o /bastion ./bastion

# Stage 3: runtime
FROM alpine:3.24
RUN apk add --no-cache ca-certificates && adduser -D -H blackbox
WORKDIR /app
COPY --from=builder /bastion .
COPY --from=web /app/web/build ./static
ENV STATIC_DIR=/app/static
USER blackbox
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
CMD ["/app/bastion"]
