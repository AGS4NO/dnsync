FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /dnsync .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates git
COPY --from=builder /dnsync /dnsync
ENTRYPOINT ["/dnsync"]
