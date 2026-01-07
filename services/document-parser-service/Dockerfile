FROM golang:1.24-alpine AS builder

WORKDIR /build

# Копируем go.mod и go.sum
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY main.go ./
COPY handlers ./handlers/
COPY parsers ./parsers/

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o document-parser-service main.go

# Финальный образ
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /build/document-parser-service .

EXPOSE 8081

CMD ["./document-parser-service"]
