FROM golang:1.25-alpine

WORKDIR /app

COPY . .

RUN go build -o /usr/bin/iridium -ldflags="-s -w" .

EXPOSE 8080

CMD ["/usr/bin/iridium"]