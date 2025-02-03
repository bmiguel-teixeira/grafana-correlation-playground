FROM golang:1.23

WORKDIR /apps
COPY . .

RUN go mod tidy
RUN go build main.go

ENTRYPOINT [ "/apps/main" ]