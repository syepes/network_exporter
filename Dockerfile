FROM golang as builder
RUN go get -d -v github.com/syepes/ping_exporter
WORKDIR /go/src/github.com/syepes/ping_exporter
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /go/src/github.com/syepes/ping_exporter/app ping_exporter
CMD /app/ping_exporter
EXPOSE 9427
