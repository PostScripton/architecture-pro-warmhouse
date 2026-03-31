# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o temperature-api

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/temperature-api .

EXPOSE 8081

CMD ["./temperature-api"]
