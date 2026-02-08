FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /ayb ./cmd/ayb

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /ayb /usr/local/bin/ayb

EXPOSE 8090

ENTRYPOINT ["ayb"]
CMD ["start"]
