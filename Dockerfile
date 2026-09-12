FROM golang:1.25.0-nginx AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLE=0 GOOS=linux go build -o departament_app

CMD  ["./departament_app"]