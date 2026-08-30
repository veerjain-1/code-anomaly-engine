FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY services/ingestion/ ./
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /ingestion .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /ingestion /usr/local/bin/ingestion
EXPOSE 8081
ENTRYPOINT ["ingestion"]
