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
FROM gcr.io/distroless/static-debian12@sha256:6447365a6337c3732f412d1b74357b30a633831955b2bc45552b0086be907687 AS runtime
COPY --from=build /app /app
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=3s CMD ["/app", "healthcheck"]
ENTRYPOINT ["/app"]
