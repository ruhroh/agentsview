# -- Frontend build --
FROM node:22-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# -- Go build --
FROM golang:1.25-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed the frontend build output
COPY --from=frontend /src/frontend/dist /src/internal/web/dist
RUN CGO_ENABLED=1 go build -tags fts5 \
    -ldflags="-s -w \
      -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo docker) \
      -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) \
      -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -trimpath -o /agentsview ./cmd/agentsview

# -- Runtime --
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=backend /agentsview /usr/local/bin/agentsview
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh
ENTRYPOINT ["/docker-entrypoint.sh"]
