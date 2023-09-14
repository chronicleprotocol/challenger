FROM golang:1.20-alpine as builder
RUN apk --no-cache add git
WORKDIR /go/src/challenger
COPY . .
RUN export CGO_ENABLED=0 \
    && mkdir -p dist \
    && go mod vendor \
    && go build -o dist/challenger ./cmd/challenger

FROM alpine:3.16
RUN apk --no-cache add ca-certificates
WORKDIR /root
COPY --from=builder /go/src/challenger/dist/ /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/challenger"]
CMD ["run"]