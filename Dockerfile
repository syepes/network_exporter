FROM golang:alpine as builder
RUN go get -d -v github.com/syepes/network_exporter
RUN cd /go/pkg/mod/github.com/syepes/network_exporter*
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /go/src/github.com/syepes/network_exporter/app network_exporter
CMD /app/network_exporter
EXPOSE 9427
