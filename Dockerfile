FROM docker.io/library/golang:1.27.4-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /build/zobotpic ./cmd/zobot

FROM docker.io/library/alpine:3.24
RUN apk add --no-cache ca-certificates \
    && addgroup -S zobotpic \
    && adduser -S zobotpic -G zobotpic
WORKDIR /app
COPY --from=builder /build/zobotpic /app/zobotpic
RUN chown zobotpic:zobotpic /app/zobotpic
USER zobotpic
ENTRYPOINT ["/app/zobotpic"]
