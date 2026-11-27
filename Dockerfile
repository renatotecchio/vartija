# syntax=docker/dockerfile:1

# --- frontend: React build ---
FROM node:24.22.3-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# --- backend: static Go binary with the frontend embedded ---
FROM golang:1.26.11-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/dist/ ./internal/adapters/httpapi/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/vartija ./cmd/vartija

# --- runtime ---
# Runs as root inside the container because the ARP sweep opens a raw
# socket. Run it with every capability dropped except NET_RAW (see
# docker-compose.yml), so root here can do nothing else.
FROM gcr.io/distroless/static-debian12
COPY --from=backend /out/vartija /vartija
EXPOSE 8090
ENTRYPOINT ["/vartija"]
