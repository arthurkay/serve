FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o serve .

FROM scratch
COPY --from=builder /build/serve /serve
EXPOSE 3000
ENTRYPOINT ["/serve"]
