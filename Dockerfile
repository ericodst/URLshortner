FROM golang:alpine3.16

COPY main.go /app/
WORKDIR /app

COPY template /template/
COPY asset/css /asset/css/
RUN ls -la /asset/css/*
COPY asset/img /asset/img/
RUN ls -la /asset/img/* .

RUN go mod init app
RUN go mod tidy
RUN go mod download

CMD go run main.go