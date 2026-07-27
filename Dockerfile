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
FROM gcr.io/distroless/static-debian12@sha256:a9fcaedd4c9b59e12dd65d954f0b5044f19b0647a8a3712e77205df9e7b102cd AS runtime
COPY --from=build /app /app
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=3s CMD ["/app", "healthcheck"]
ENTRYPOINT ["/app"]
