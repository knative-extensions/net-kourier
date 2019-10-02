FROM golang:1.13
WORKDIR /app/
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o kourier cmd/kourier/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
USER 1001
COPY --from=0 /app/kourier /app/kourier
EXPOSE 18000 19000
ENTRYPOINT ["/app/kourier"]
