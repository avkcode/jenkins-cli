FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /jc .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /jc /usr/local/bin/jc
ENTRYPOINT ["/usr/local/bin/jc"]
