FROM golang:alpine as builder
RUN go install -v github.com/syepes/network_exporter@latest
RUN cd /go/pkg/mod/github.com/syepes/network_exporter* && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
RUN apk --no-cache add ca-certificates libcap iproute2 && mkdir -p /app/cfg
WORKDIR /app
COPY --from=builder /go/pkg/mod/github.com/syepes/network_exporter*/app network_exporter
RUN setcap 'cap_net_raw,cap_net_admin+eip' /app/network_exporter
CMD /app/network_exporter
EXPOSE 9427
