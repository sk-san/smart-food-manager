# --- build stage ---
FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# Two binaries from one image: the API, and the migrator the platform runs as
# a release step. Shipping them together is what guarantees the schema and the
# code that expects it are always the same build.
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api && \
    CGO_ENABLED=0 go build -o /bin/migrate ./cmd/migrate

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/api /api
COPY --from=build /bin/migrate /migrate
EXPOSE 8080
ENTRYPOINT ["/api"]
