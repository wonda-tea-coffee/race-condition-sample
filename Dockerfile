FROM golang:1.23

WORKDIR /app
COPY . .
RUN go mod tidy

CMD ["go", "run", "."]
