# Build stage
FROM golang:1.26-bookworm AS build
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /app ./cmd/blogflow

# Runtime stage — distroless, rootless, no shell
# gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12@sha256:9c346e4be81b5ca7ff31a0d89eaeade58b0f95cfd3baed1f36083ddb47ca3160 AS runtime
COPY --from=build /app /app
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=3s CMD ["/app", "healthcheck"]
ENTRYPOINT ["/app"]
