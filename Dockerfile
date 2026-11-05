FROM node:22-alpine AS frontend-build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
ENV CI=true
RUN npm run build

FROM golang:1.27-alpine AS backend-build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/socialsphere .
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend-build --chown=65532:65532 /out/ /app/
COPY --from=frontend-build --chown=65532:65532 /app/build/ /app/public/
ENV HOST=0.0.0.0 PORT=8080 STORAGE_MODE=local DATA_DIR=/app/data FRONTEND_DIR=/app/public
EXPOSE 8080
ENTRYPOINT ["/app/socialsphere"]
