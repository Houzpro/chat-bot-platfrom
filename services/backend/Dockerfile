FROM golang:1.24-alpine AS builder

WORKDIR /build

# Копируем go.mod и go.sum
COPY go.mod go.sum ./

# Копируем все исходники (включая packages)
COPY . .

# Обновляем зависимости и собираем
RUN go mod tidy && go mod download && go mod verify
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o backend main.go

# Финальный образ
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /build/backend .

EXPOSE 8080

CMD ["./backend"]
