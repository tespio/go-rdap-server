FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o rdapd ./cmd/rdapd

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S rdap && adduser -S rdap -G rdap

WORKDIR /app
COPY --from=builder /build/rdapd .
COPY config.yaml .

RUN mkdir -p /data && chown -R rdap:rdap /app /data

USER rdap

EXPOSE 8443 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9090/healthz || exit 1

ENTRYPOINT ["./rdapd", "-config", "config.yaml"]
