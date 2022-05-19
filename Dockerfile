FROM golang:1.18.2

WORKDIR /workspace

RUN go mod download && go mod verify

RUN go build -v -o /usr/local/bin/pooltoprom ./...

CMD ["pooltoprom"]
