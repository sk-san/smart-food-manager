# --- build stage ---
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
