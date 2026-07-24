# Stage 1: Build Go App
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bsm-server main.go

# Stage 2: Production Minimal Image
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /app/bsm-server /app/bsm-server
COPY --from=builder /app/static /app/static
COPY --from=builder /app/scripts /app/scripts

EXPOSE 8080
ENV PORT=8080

CMD ["/app/bsm-server"]
