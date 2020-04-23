# ping_exporter

ICMP echo request ("ping") & MTR & TCP Port Prometheus exporter

This exporter gathers either ICMP, MTR or TCP Port stats and exports them via HTTP for Prometheus consumption.

## Features

- IPv4 & IPv6 support
- Configuration reloading (By interval or OS signal)
- Dynamically Add or Remove targets without affecting the currently running tests
- Targets can be executed on all hosts or a list of specified ones `probe`
- Configurable logging levels and format (text or json)

### Exported metrics

- `ping_up`                          Exporter state
- `ping_targets`                     Number of active targets
- `ping_status`:                     Ping Status
- `ping_rtt_seconds{type=best}`:     Best round trip time in seconds
- `ping_rtt_seconds{type=worst}`:    Worst round trip time in seconds
- `ping_rtt_seconds{type=mean}`:     Mean round trip time in seconds
- `ping_rtt_seconds{type=sum}`:      Sum round trip time in seconds
- `ping_rtt_seconds{type=stddev}`:   Standard deviation in seconds
- `ping_loss_percent`:               Packet loss in percent

---

- `mtr_up`                           Exporter state
- `mtr_targets`                      Number of active targets
- `mtr_hops`                         Number of route hops
- `mtr_rtt_seconds{type=last}`:      Last round trip time in seconds
- `mtr_rtt_seconds{type=best}`:      Best round trip time in seconds
- `mtr_rtt_seconds{type=worst}`:     Worst round trip time in seconds
- `mtr_rtt_seconds{type=mean}`:      Mean round trip time in seconds
- `mtr_rtt_seconds{type=sum}`:       Sum round trip time in seconds
- `mtr_rtt_seconds{type=stddev}`:    Standard deviation time in seconds
- `mtr_rtt_seconds{type=loss}`:      Packet loss in percent

---

- `tcp_up`                           Exporter state
- `tcp_targets`                      Number of active targets
- `tcp_connection_status`            Connection Status
- `tcp_connection_seconds`           Connection time in seconds

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
$ ls -l artifacts/ping_exporter_*64
# If you want to run it with a non root user
$ sudo setcap cap_net_raw=+ep artifacts/ping_exporter_linux_amd64/ping_exporter
```

### Building with Docker

To run the ping_exporter as a Docker container by builing your own image or using <https://hub.docker.com/r/syepes/ping_exporter>

```console
docker build -t syepes/ping_exporter .
docker run -p 9427:9427 -v ./ping_exporter.yml:/ping_exporter.yml:ro --name ping_exporter syepes/ping_exporter
```

## Configuration

To see all available configuration flags:

```console
./ping_exporter -h
```

Most of the configuration is set in the YAML based config file:

```yaml
conf:
  refresh: 15m

icmp:
  interval: 3s
  timeout: 1s
  count: 10

tcp:
  interval: 3s
  timeout: 1s

mtr:
  interval: 3s
  timeout: 500ms
  max-hops: 30
  snt-size: 3

targets:
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
  - name: internal
    host: 192.168.0.1
    type: ICMP
    probe:
      - hostname1
      - hostname2
```

**Note:** domains are resolved (regularly) to their corresponding A and AAAA records (IPv4 and IPv6).
By default, `ping_exporter` uses the system resolver to translate domain names to IP addresses.

## Grafana

[Grafana Dashboard](https://github.com/syepes/ping_exporter/blob/master/dist/ping_exporter.json)

![gradana](https://raw.githubusercontent.com/syepes/ping_exporter/master/dist/ping_exporter.png)

## Contribute

If you have any idea for an improvement or find a bug do not hesitate in opening an issue, just simply fork and create a pull-request to help improve the exporter.

## License

All content is distributed under the [Apache 2.0 License](http://www.apache.org/licenses/LICENSE-2.0)
Copyright &copy; 2020, [Sebastian YEPES](mailto:syepes@gmail.com)
