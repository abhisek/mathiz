# Multi-stage build: React SPA → Go binary (SPA embedded) → minimal runtime.
# Produces the single self-contained `mathiz serve` image used by
# docker-compose's `mathiz` service.

# --- Stage 1: build the web SPA ---
FROM node:20-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# vite.config.ts emits to ../internal/saas/webui/dist — create the target tree.
RUN mkdir -p /src/internal/saas/webui && npm run build

# --- Stage 2: build the Go binary with the SPA embedded ---
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/saas/webui/dist internal/saas/webui/dist
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/mathiz .

# --- Stage 3: runtime ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 mathiz
USER mathiz
COPY --from=build /out/mathiz /usr/local/bin/mathiz
EXPOSE 8080
ENTRYPOINT ["mathiz"]
CMD ["serve"]
