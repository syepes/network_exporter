FROM golang:alpine as builder
RUN go install -v github.com/syepes/network_exporter
RUN cd /go/pkg/mod/github.com/syepes/network_exporter* && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /go/pkg/mod/github.com/syepes/network_exporter*/app network_exporter
CMD /app/network_exporter
EXPOSE 9427
