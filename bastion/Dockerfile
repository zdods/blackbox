# Stage 1: build web app
FROM node:20-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: build bastion binary
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY pkg/ pkg/
COPY bastion/ bastion/
RUN CGO_ENABLED=0 go build -o /bastion ./bastion

# Stage 3: runtime
FROM alpine:3.21
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
