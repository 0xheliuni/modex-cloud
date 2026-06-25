# ---- Frontend build stage ----
FROM node:20-alpine AS web
WORKDIR /web
ARG NPM_REGISTRY=https://registry.npmmirror.com
COPY web/package.json web/package-lock.json* ./
RUN npm config set registry ${NPM_REGISTRY} && npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Backend build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Bring in the freshly built SPA so go:embed picks it up.
COPY --from=web /web/dist ./web/dist
# CGO disabled: glebarez/sqlite is pure-Go, so the binary is fully static.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /modex-cloud .

# ---- Runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /modex-cloud /app/modex-cloud
# Data dir for the SQLite file (mounted as a volume in docker-compose). Owned by
# appuser so the non-root process can create/write the database.
RUN mkdir -p /data && chown appuser:appuser /data
VOLUME ["/data"]
USER appuser
EXPOSE 3000
ENTRYPOINT ["/app/modex-cloud"]
