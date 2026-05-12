FROM golang:1.26.3-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o judge-executor .

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache docker-cli ca-certificates

COPY --from=builder /app/judge-executor ./judge-executor
COPY --from=builder /app/docker ./docker

EXPOSE 50051

CMD ["./judge-executor"]
