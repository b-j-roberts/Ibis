# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/ibis ./cmd/ibis

# Stage 2: Runtime (distroless)
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/ibis /app/ibis

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["/app/ibis"]
CMD ["run"]
