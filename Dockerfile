FROM golang:1.13
WORKDIR /app/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM envoyproxy/envoy-alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY envoy-bootstrap.yaml /
COPY docker-entrypoint.sh /
COPY --from=0 /app/app /root/app
