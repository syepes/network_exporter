# network_exporter

[![Github Action](https://github.com/syepes/network_exporter/workflows/build/badge.svg)](https://github.com/syepes/network_exporter/actions)
[![Docker Pulls](https://img.shields.io/docker/pulls/syepes/network_exporter.svg?maxAge=604800)](https://hub.docker.com/r/syepes/network_exporter)

ICMP & MTR & TCP Port & HTTP Get Prometheus exporter

This exporter gathers either ICMP, MTR, TCP Port or HTTP Get stats and exports them via HTTP for Prometheus consumption.

![gradana](https://raw.githubusercontent.com/syepes/network_exporter/master/dist/network_exporter.gif)

## Features

- IPv4 & IPv6 support
- Configuration reloading (By interval or OS signal)
- Dynamically Add or Remove targets without affecting the currently running tests
- Targets can be executed on all hosts or a list of specified ones `probe`
- Configurable logging levels and format (text or json)
- Configurable DNS Server

### Exported metrics

- `ping_up`                                        Exporter state
- `ping_targets`                                   Number of active targets
- `ping_status`:                                   Ping Status
- `ping_rtt_seconds{type=best}`:                   Best round trip time in seconds
- `ping_rtt_seconds{type=worst}`:                  Worst round trip time in seconds
- `ping_rtt_seconds{type=mean}`:                   Mean round trip time in seconds
- `ping_rtt_seconds{type=sum}`:                    Sum round trip time in seconds
- `ping_rtt_seconds{type=sd}`:                     Squared deviation in seconds
- `ping_rtt_seconds{type=usd}`:                    Standard deviation without correction in seconds
- `ping_rtt_seconds{type=csd}`:                    Standard deviation with correction (Bessel's) in seconds
- `ping_rtt_seconds{type=range}`:                  Range in seconds
- `ping_loss_percent`:                             Packet loss in percent

---

- `mtr_up`                                         Exporter state
- `mtr_targets`                                    Number of active targets
- `mtr_hops`                                       Number of route hops
- `mtr_rtt_seconds{type=last}`:                    Last round trip time in seconds
- `mtr_rtt_seconds{type=best}`:                    Best round trip time in seconds
- `mtr_rtt_seconds{type=worst}`:                   Worst round trip time in seconds
- `mtr_rtt_seconds{type=mean}`:                    Mean round trip time in seconds
- `mtr_rtt_seconds{type=sum}`:                     Sum round trip time in seconds
- `mtr_rtt_seconds{type=sd}`:                      Squared deviation in seconds
- `mtr_rtt_seconds{type=usd}`:                     Standard deviation without correction in seconds
- `mtr_rtt_seconds{type=csd}`:                     Standard deviation with correction (Bessel's) in seconds
- `mtr_rtt_seconds{type=range}`:                   Range in seconds
- `mtr_rtt_seconds{type=loss}`:                    Packet loss in percent

---

- `tcp_up`                                         Exporter state
- `tcp_targets`                                    Number of active targets
- `tcp_connection_status`                          Connection Status
- `tcp_connection_seconds`                         Connection time in seconds

---

- `http_get_up`                                    Exporter state
- `http_get_targets`                               Number of active targets
- `http_get_status`                                HTTP Status Code and Connection Status
- `http_get_content_bytes`                         HTTP Get Content Size in bytes
- `http_get_seconds{type=DNSLookup}`:              DNSLookup connection drill down time in seconds
- `http_get_seconds{type=TCPConnection}`:          TCPConnection connection drill down time in seconds
- `http_get_seconds{type=TLSHandshake}`:           TLSHandshake connection drill down time in seconds
- `http_get_seconds{type=TLSEarliestCertExpiry}`:  TLSEarliestCertExpiry cert expiration time in epoch
- `http_get_seconds{type=TLSLastChainExpiry}`:     TLSLastChainExpiry cert expiration time in epoch
- `http_get_seconds{type=ServerProcessing}`:       ServerProcessing connection drill down time in seconds
- `http_get_seconds{type=ContentTransfer}`:        ContentTransfer connection drill down time in seconds
- `http_get_seconds{type=Total}`:                  Total connection time in seconds

Each metric contains the labels:

- `name` (ALL: The target name)
- `target` (ALL: the target IP Address)
- `port` (TCP: the target TCP Port)
- `ttl` (MTR: time to live)
- `path` (MTR: traceroute IP)

## Building and running the software

### Local Build

```console
$ goreleaser release --skip-publish --snapshot --rm-dist
$ ls -l artifacts/network_exporter_*6?
# If you want to run it with a non root user
$ sudo setcap cap_net_raw=+ep artifacts/network_exporter_linux_amd64/network_exporter
```

### Building with Docker

To run the network_exporter as a Docker container by builing your own image or using <https://hub.docker.com/r/syepes/network_exporter>

```console
docker build -t syepes/network_exporter .
# Default mode
docker run -p 9427:9427 -v $PWD/network_exporter.yml:/network_exporter.yml:ro --name network_exporter syepes/network_exporter
# Debug level
docker run -p 9427:9427 -v $PWD/network_exporter.yml:/network_exporter.yml:ro --name network_exporter syepes/network_exporter /app/network_exporter --log.level=debug
```

## Configuration

To see all available configuration flags:

```console
./network_exporter -h
```

Most of the configuration is set in the YAML based config file:

```yaml
conf:
  refresh: 15m
  nameserver: 192.168.0.1:53

icmp:
  interval: 3s
  timeout: 1s
  count: 6

mtr:
  interval: 3s
  timeout: 500ms
  max-hops: 30
  count: 6

tcp:
  interval: 3s
  timeout: 1s

http_get:
  interval: 15m
  timeout: 5s

targets:
  - name: internal
    host: 192.168.0.1
    type: ICMP
    probe:
      - hostname1
      - hostname2
  - name: google-dns1
    host: 8.8.8.8
    type: ICMP
  - name: google-dns2
    host: 8.8.4.4
    type: MTR
  - name: cloudflare-dns
    host: 1.1.1.1
    type: ICMP+MTR
  - name: cloudflare-dns-https
    host: 1.1.1.1:443
    type: TCP
  - name: download-file-64M
    host: http://test-debit.free.fr/65536.rnd
    type: HTTPGet
  - name: download-file-64M-proxy
    host: http://test-debit.free.fr/65536.rnd
    type: HTTPGet
    proxy: http://localhost:3128
```

**Note:** Domain names are resolved (regularly) to their corresponding A and AAAA records (IPv4 and IPv6).
By default if not configured, `network_exporter` uses the system resolver to translate domain names to IP addresses.
You can alos override the DNS resolver address by specifying the `conf.nameserver` configuration setting.

## Deployment

This deployment example will permit you to have as many Ping Stations as you need (LAN or WIFI) devices but at the same time decoupling the data collection from the storage and visualization.
This [docker compose](https://docs.docker.com/compose/) will deploy and configure all the components plus setup Grafana with the Datasource and [Dashboard](https://github.com/syepes/network_exporter/blob/master/dist/deploy/cfg/provisioning/dashboards/network_exporter.json)

[Deployment example](https://github.com/syepes/network_exporter/blob/master/dist/deploy/)

![Deployment architecture](https://raw.githubusercontent.com/syepes/network_exporter/master/dist/deployment.jpg)

## Contribute

If you have any idea for an improvement or find a bug do not hesitate in opening an issue, just simply fork and create a pull-request to help improve the exporter.

## License

All content is distributed under the [Apache 2.0 License](http://www.apache.org/licenses/LICENSE-2.0)
Copyright &copy; 2020, [Sebastian YEPES](mailto:syepes@gmail.com)
