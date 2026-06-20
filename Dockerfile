FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /initdb ./cmd/initdb
RUN CGO_ENABLED=0 GOOS=linux go build -o /chaos-monkey ./cmd/chaos-monkey

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /server /app/server
COPY --from=builder /initdb /app/initdb

RUN mkdir -p /app/data && /app/initdb

EXPOSE 3000

ENV PORT=3000
ENV DD_SERVICE=order-service
ENV DD_TRACE_ENABLED=true
ENV DB_PATH=/app/data/app.db

CMD ["/app/server"]
