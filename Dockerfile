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
FROM gcr.io/distroless/static-debian12@sha256:22fd79fd75eab2372585b44517f8a094349938919dc613aafc37e4bdc9967c82 AS runtime
COPY --from=build /app /app
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=3s CMD ["/app", "healthcheck"]
ENTRYPOINT ["/app"]
