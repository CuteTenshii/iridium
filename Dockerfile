FROM golang:1.25-alpine

WORKDIR /app

COPY . .

RUN go build -o iridium -ldflags="-s -w" .

EXPOSE 8080

CMD ["./iridium"]