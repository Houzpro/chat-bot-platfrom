FROM golang:1.25-alpine AS builder

WORKDIR /build

# Копируем go.mod и go.sum
COPY go.mod go.sum ./

# Копируем все исходники (включая packages)
COPY . .

# Собираем. Не запускаем `go mod tidy` в Dockerfile — это может изменить
# go.mod/go.sum от того, что зафиксировано в репозитории, и привести к
# непредсказуемым билдам. Доверяем go.sum, который зафиксирован при
# разработке. `go mod download` достаточно для получения зависимостей.
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o backend main.go

# Финальный образ
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /build/backend .

EXPOSE 8080

CMD ["./backend"]
