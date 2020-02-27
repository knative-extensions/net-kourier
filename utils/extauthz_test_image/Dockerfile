FROM golang:1.13
WORKDIR /app/
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod vendor -a -installsuffix cgo -o extauthz main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
USER 1001
COPY --from=0 /app/extauthz /app/extauthz
EXPOSE 6000
ENTRYPOINT ["/app/extauthz"]
