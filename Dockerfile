FROM golang:1.25-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o go-ewallet-backend .
EXPOSE 9090
CMD ["./go-ewallet-backend"]
