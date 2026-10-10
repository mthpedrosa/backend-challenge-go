# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27.2-alpine AS build
WORKDIR /src

# Cache dependencies separately from source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wallet ./cmd/wallet

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/wallet /usr/local/bin/wallet
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/wallet"]
