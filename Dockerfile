FROM docker.io/library/golang:1.23.1 as builder
RUN mkdir /build
COPY . /build
WORKDIR /build
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o zobotpic .

FROM docker.io/library/golang:alpine
RUN mkdir /app
COPY --from=builder /build/zobotpic /app
WORKDIR /app
RUN addgroup -S zobotpic && adduser -S zobotpic -G zobotpic
RUN chown -R zobotpic:zobotpic /app
USER zobotpic
ENTRYPOINT ["/app/zobotpic"]