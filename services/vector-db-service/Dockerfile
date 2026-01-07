FROM golang:1.24-alpine AS builder

WORKDIR /build

# Копируем go.mod и go.sum
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY main.go ./
COPY handlers ./handlers/
COPY models ./models/
COPY services ./services/

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vector-db-service main.go

# Финальный образ
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /build/vector-db-service .

EXPOSE 8082

CMD ["./vector-db-service"]
