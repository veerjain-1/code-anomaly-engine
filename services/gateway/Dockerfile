FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY services/gateway/ ./
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /gateway .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /gateway /usr/local/bin/gateway
EXPOSE 8080
ENTRYPOINT ["gateway"]
