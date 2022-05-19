FROM golang:1.18.2

RUN go mod download && go mod verify

RUN go build -v -o /usr/local/bin/pooltoprom ./...

CMD ["pooltoprom"]
