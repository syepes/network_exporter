# network_exporter

[![Github Action](https://github.com/syepes/network_exporter/workflows/build/badge.svg)](https://github.com/syepes/network_exporter/actions)
[![Docker Pulls](https://img.shields.io/docker/pulls/syepes/network_exporter.svg?maxAge=604800)](https://hub.docker.com/r/syepes/network_exporter)

ICMP & MTR & TCP Port & HTTP Get Prometheus exporter

This exporter gathers either ICMP, MTR, TCP Port or HTTP Get stats and exports them via HTTP for Prometheus consumption.

![grafana](https://raw.githubusercontent.com/syepes/network_exporter/master/dist/network_exporter.gif)

## Features

- IPv4 & IPv6 support
- Configuration reloading (By interval or OS signal)
- Dynamically Add or Remove targets without affecting the currently running tests
- Automatic update of the target IP when the DNS resolution changes
- Targets can be executed on all hosts or a list of specified ones `probe`
- Extra labels when defining targets
- Configurable logging levels and format (text or json)
- Configurable DNS Server
- Configurable Source IP per target `source_ip` (optional), The IP has to be configured on one of the instance's interfaces
- **Configurable concurrency control per target type**
- **High-performance optimizations**
- **Startup jitter to prevent thundering herd**
- **Configurable ICMP payload size** for PING and MTR probes
- **TCP-based MTR traceroute** option for firewall-friendly network path discovery

## Performance and Scaling

The network_exporter is designed to efficiently handle large numbers of targets with built-in performance optimizations

### Scaling Limits

With default settings (`--max-concurrent-jobs=3`) and built-in optimizations:

| Target Type | Recommended Limit | Notes |
|-------------|------------------|-------|
| **PING** | 10,000 - 15,000 targets | Limited by ICMP ID counter (~65,500 concurrent operations) |
| **MTR** | 1,000 - 1,500 targets | MTR uses multiple ICMP IDs per operation |
| **TCP** | 15,000 - 25,000 targets | Optimized DNS handling improves scaling |
| **HTTPGet** | 10,000 - 15,000 targets | Connection pooling enables better scaling |

### Performance Tuning

#### Understanding Concurrency

The `--max-concurrent-jobs` parameter controls **per-target** concurrency, not total system concurrency.

**Formula:** `Total System Concurrency = Number of Targets × max-concurrent-jobs`

**Why lower per-target concurrency for large deployments?**

| Targets | max-concurrent-jobs | Total Concurrent Operations | Resource Impact |
|---------|---------------------|----------------------------|--------------------------------------|
| 100 | 5 | 100 × 5 = **500** | ✓ Low - system handles easily |
| 100 | 2 | 100 × 2 = **200** | ✓ Low - but slower per target |
| 1,000 | 5 | 1,000 × 5 = **5,000** | ⚠️ Moderate - manageable with optimizations |
| 1,000 | 3 | 1,000 × 3 = **3,000** | ✓ Low-Moderate - recommended |
| 5,000 | 3 | 5,000 × 3 = **15,000** | ⚠️ High - possible but use monitoring |
| 5,000 | 2 | 5,000 × 2 = **10,000** | ✓ Moderate - optimized for scale |
| 15,000 | 2 | 15,000 × 2 = **30,000** | ✓ High but manageable - was not feasible before |

**The Tradeoff:**
- **Higher per-target concurrency** = Faster individual target probing, but higher total resource usage
- **Lower per-target concurrency** = Slower individual target probing, but prevents resource exhaustion at scale

#### Concurrency Recommendations

With built-in optimizations, the exporter can handle larger deployments more efficiently:

```bash
# Default: 3 concurrent operations per target (100 targets × 3 = 300 operations)
./network_exporter --max-concurrent-jobs=3

# Small deployments (<100 targets): Use higher per-target concurrency
# Example: 50 targets × 5 = 250 total concurrent operations
./network_exporter --max-concurrent-jobs=5

# Medium deployments (100-1000 targets): Use default
# Example: 500 targets × 3 = 1,500 total concurrent operations
./network_exporter --max-concurrent-jobs=3

# Large deployments (1000-5000 targets): Use default or slightly lower
# Example: 3,000 targets × 3 = 9,000 total concurrent operations
./network_exporter --max-concurrent-jobs=3

# Very large deployments (>5000 targets): Use lower per-target concurrency
# Example: 15,000 targets × 2 = 30,000 total concurrent operations
# Optimizations make this feasible where it wasn't before
./network_exporter --max-concurrent-jobs=2
```

#### Resource Requirements

With built-in optimizations, resource requirements are reduced:

- **Memory:** ~50-100MB baseline + ~0.8-3KB per target (reduced from 1-5KB due to optimizations)
- **CPU:** Mostly I/O bound, 25-40% more efficient with optimizations
- **File Descriptors:** Set `ulimit -n` to at least `(targets × max-concurrent-jobs) + 1000`

**Example for 5,000 targets:**
```bash
# Calculate file descriptor needs: 5,000 targets × 2 jobs = 10,000 + buffer
ulimit -n 20000

# Run with optimized settings (10,000 total concurrent operations)
./network_exporter --max-concurrent-jobs=2
```

**Example for 15,000 targets (with optimizations):**
```bash
# Higher scale now possible with built-in optimizations
ulimit -n 40000

# Run with conservative settings for large scale
./network_exporter --max-concurrent-jobs=2
```

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
- `ping_rtt_snt_count`:                            Packet sent count total
- `ping_rtt_snt_fail_count`:                       Packet sent fail count total
- `ping_rtt_snt_seconds`:                          Packet sent time total in seconds
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
- `mtr_rtt_snt_count`:                             Packet sent count total
- `mtr_rtt_snt_fail_count`:                        Packet sent fail count total
- `mtr_rtt_snt_seconds`:                           Packet sent time total in seconds

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

Each metric contains the below labels and additionally the ones added in the configuration file.

- `name` (ALL: The target name)
- `target` (ALL: The target defined Hostname or IP)
- `target_ip` (ALL: The target resolved IP Address)
- `source_ip` (ALL: The source IP Address)
- `port` (TCP: The target TCP Port)
- `ttl` (MTR: Time to live)
- `path` (MTR: Traceroute IP)

## Building and running the software

### Prerequisites for Linux

The process must run with the necesary linux or docker previlages to be able to perform the necesary tests

```bash
apt update
apt install docker
apt install docker.io
touch network_exporter.yml
```

### Local Build

```bash
$ goreleaser release --skip=publish --snapshot --clean
$ ls -l artifacts/network_exporter_*6?
# If you want to run it with a non root user
$ sudo setcap 'cap_net_raw,cap_net_admin+eip' artifacts/network_exporter_linux_amd64/network_exporter
```

### Building with Docker

To run the network_exporter as a Docker container by builing your own image or using <https://hub.docker.com/r/syepes/network_exporter>

```bash
docker build -t syepes/network_exporter .

# Default mode
docker run --privileged --cap-add NET_ADMIN --cap-add NET_RAW -p 9427:9427 \
  -v $PWD/network_exporter.yml:/app/cfg/network_exporter.yml:ro \
  --name network_exporter syepes/network_exporter

# Debug level
docker run --privileged --cap-add NET_ADMIN --cap-add NET_RAW -p 9427:9427 \
  -v $PWD/network_exporter.yml:/app/cfg/network_exporter.yml:ro \
  --name network_exporter syepes/network_exporter \
  /app/network_exporter --log.level=debug

# Large deployment (e.g., 5000 targets): Lower per-target concurrency
# Total concurrency: 5000 targets × 2 = 10,000 concurrent operations
# Built-in optimizations reduce resource usage by 25-40%
docker run --privileged --cap-add NET_ADMIN --cap-add NET_RAW -p 9427:9427 \
  -v $PWD/network_exporter.yml:/app/cfg/network_exporter.yml:ro \
  --ulimit nofile=20000:20000 \
  --name network_exporter syepes/network_exporter \
  /app/network_exporter --max-concurrent-jobs=2

# Very large deployment (e.g., 15000 targets): Now possible with optimizations
# Total concurrency: 15000 targets × 2 = 30,000 concurrent operations
docker run --privileged --cap-add NET_ADMIN --cap-add NET_RAW -p 9427:9427 \
  -v $PWD/network_exporter.yml:/app/cfg/network_exporter.yml:ro \
  --ulimit nofile=40000:40000 \
  --name network_exporter syepes/network_exporter \
  /app/network_exporter --max-concurrent-jobs=2

# Small deployment (e.g., 50 targets): Higher per-target concurrency
# Total concurrency: 50 targets × 5 = 250 concurrent operations
docker run --privileged --cap-add NET_ADMIN --cap-add NET_RAW -p 9427:9427 \
  -v $PWD/network_exporter.yml:/app/cfg/network_exporter.yml:ro \
  --name network_exporter syepes/network_exporter \
  /app/network_exporter --max-concurrent-jobs=5
```

## Configuration

### Command-Line Flags

To see all available configuration flags:

```bash
./network_exporter -h
```

**Key flags:**
- `--config.file` - Path to the YAML configuration file (default: `/app/cfg/network_exporter.yml`)
- `--max-concurrent-jobs` - Maximum concurrent probe operations per target (default: `3`)
- `--ipv6` - Enable IPv6 support (default: `true`)
- `--web.listen-address` - Address to listen on for HTTP requests (default: `:9427`)
- `--log.level` - Logging level: debug, info, warn, error (default: `info`)
- `--log.format` - Logging format: logfmt, json (default: `logfmt`)
- `--profiling` - Enable profiling endpoints (pprof + fgprof) (default: `false`)

### YAML Configuration

The configuration (YAML) is mainly separated into three sections Main, Protocols and Targets.
The file `network_exporter.yml` can be either edited before building the docker container or changed it runtime.

```yaml
# Main Config
conf:
  refresh: 15m
  nameserver: 192.168.0.1:53 # Optional
  nameserver_timeout: 250ms # Optional

# Specific Protocol settings
icmp:
  interval: 3s
  timeout: 1s
  count: 6
  payload_size: 56  # Optional, ICMP payload size in bytes (default: 56)

mtr:
  interval: 3s
  timeout: 500ms
  max-hops: 30
  count: 6
  payload_size: 56  # Optional, ICMP payload size in bytes (default: 56)
  protocol: icmp    # Optional, Protocol to use: "icmp" or "tcp" (default: "icmp")
  tcp_port: 80      # Optional, Default port for TCP traceroute (default: "80")

tcp:
  interval: 3s
  timeout: 1s

http_get:
  interval: 15m
  timeout: 5s

# Target list and settings
targets:
  - name: internal
    host: 192.168.0.1
    type: ICMP
    probe:
      - hostname1
      - hostname2
    labels:
      dc: home
      rack: a1
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
    source_ip: 192.168.1.1
    type: TCP
  - name: download-file-64M
    host: http://test-debit.free.fr/65536.rnd
    type: HTTPGet
  - name: download-file-64M-proxy
    host: http://test-debit.free.fr/65536.rnd
    type: HTTPGet
    proxy: http://localhost:3128
```

**Payload Size**

The `payload_size` parameter (optional) configures the ICMP packet payload size in bytes for ICMP and MTR probes. The default is **56 bytes**, which matches the standard `ping` and `traceroute` utilities.

- **Minimum:** 4 bytes (space for sequence number)
- **Default:** 56 bytes (standard ping/traceroute payload)
- **Maximum:** Limited by MTU (typically 1472 bytes for IPv4, 1452 for IPv6)

**Use cases:**
- **Path MTU Discovery:** Test different packet sizes to identify MTU issues
- **Network Stress Testing:** Use larger payloads to simulate higher bandwidth usage
- **Performance Testing:** Measure latency with varying packet sizes

```yaml
icmp:
  interval: 3s
  timeout: 1s
  count: 6
  payload_size: 56    # Standard size (default)

mtr:
  interval: 3s
  timeout: 500ms
  max-hops: 30
  count: 6
  payload_size: 1400  # Larger payload for MTU testing
```

**MTR Protocol Selection**

The `protocol` parameter (optional) allows you to choose between ICMP and TCP for MTR (traceroute) operations. The default is **icmp**, which is the standard traceroute protocol.

**ICMP Protocol (default):**
```yaml
mtr:
  protocol: icmp     # Standard ICMP Echo traceroute
  payload_size: 56
```

**TCP Protocol:**
```yaml
mtr:
  protocol: tcp      # TCP SYN-based traceroute
  tcp_port: 443     # Default port for TCP traceroute
```

**Key Differences:**

| Feature | ICMP Traceroute | TCP Traceroute |
|---------|----------------|----------------|
| **Protocol** | ICMP Echo Request | TCP SYN packets |
| **Firewall Bypass** | Often blocked by firewalls | More likely to pass through firewalls |
| **Path Accuracy** | May take different path | Follows actual application traffic path |
| **Port Required** | No | Yes (default: 80) |
| **Use Case** | General network diagnosis | Testing connectivity to specific services |

**TCP Traceroute Benefits:**
- **Firewall-Friendly:** Many firewalls block ICMP/UDP but allow TCP traffic
- **Real-World Path:** Tests the actual path TCP connections will take
- **Service-Specific:** Can test connectivity to specific ports (80, 443, etc.)

**TCP Port Configuration:**

You can specify the port in two ways:

1. **Global default (tcp_port in config):**
   ```yaml
   mtr:
     protocol: tcp
     tcp_port: 443    # All MTR targets use port 443 by default

   targets:
     - name: google-https
       host: google.com
       type: MTR
   ```

2. **Per-target port (in host string):**
   ```yaml
   mtr:
     protocol: tcp
     tcp_port: 80     # Default fallback

   targets:
     - name: web-service
       host: example.com:443    # Explicit port 443
       type: MTR

     - name: api-service
       host: api.example.com:8080   # Explicit port 8080
       type: MTR

     - name: default-service
       host: service.com       # Uses tcp_port default (80)
       type: MTR
   ```

**Example Configurations:**

```yaml
# ICMP traceroute (default behavior)
mtr:
  interval: 5s
  timeout: 4s
  max-hops: 30
  count: 10
  protocol: icmp

targets:
  - name: google-dns
    host: 8.8.8.8
    type: MTR

# TCP traceroute to HTTPS services
mtr:
  interval: 5s
  timeout: 4s
  max-hops: 30
  count: 10
  protocol: tcp
  tcp_port: 443

targets:
  - name: website-https
    host: example.com:443
    type: MTR

  - name: api-server
    host: api.example.com:8443
    type: MTR

# Mixed: Use ICMP but support custom ports per target
mtr:
  interval: 5s
  timeout: 4s
  max-hops: 30
  count: 10
  protocol: tcp
  tcp_port: 80      # Default

targets:
  - name: web-http
    host: example.com        # Uses port 80
    type: MTR

  - name: web-https
    host: example.com:443    # Uses port 443
    type: MTR
```

**Source IP**

`source_ip` parameter will try to assign IP for request sent to specific target. This IP has to be configure on one of the interfaces of the OS.
Supported for all types of the checks

```yaml
  - name: server3.example.com:9427
    host: server3.example.com:9427
    type: TCP
    source_ip: 192.168.1.1
```

**Note:** Domain names are resolved (regularly) to their corresponding A and AAAA records (IPv4 and IPv6).
By default if not configured, `network_exporter` uses the system resolver to translate domain names to IP addresses.
You can also override the DNS resolver address by specifying the `conf.nameserver` configuration setting.

**[SRV records](https://en.wikipedia.org/wiki/SRV_record):**
If the host field of a target contains a SRV record with the format `_<service>._<protocol>.<domain>` it will be resolved, all it's A records will be added (dynamically) as separate targets with name and host of the this A record.
Every field of the parent target with a SRV record will be inherited by sub targets except `name` and `host`

SRV record supported for ICMP/MTR/TCP target types.
TCP SRV record specifcs:

- Target type should be `TCP` and `_protocol` part in the SRV record should be `_tcp` as well
- Port will be taken from the 3rd number, just before the hostname

TCP SRV example

```console
_connectivity-check._tcp.example.com. 86400 IN SRV 10 5 80 server.example.com.
_connectivity-check._tcp.example.com. 86400 IN SRV 10 5 443 server2.example.com.
_connectivity-check._tcp.example.com. 86400 IN SRV 10 5 9247 server3.example.com.
```

ICMP SRV example

```console
_connectivity-check._icmp.example.com. 86400 IN SRV 10 5 8 server.example.com.
_connectivity-check._icmp.example.com. 86400 IN SRV 10 5 8 server2.example.com.
_connectivity-check._icmp.example.com. 86400 IN SRV 10 5 8 server3.example.com.
```

Configuration reference

```yaml
  - name: test-srv-record
    host: _connectivity-check._icmp.example.com
    type: ICMP
  - name: test-srv-record
    host: _connectivity-check._tcp_.example.com
    type: TCP
```

Will be resolved to 3 separate targets:

```yaml
  - name: server.example.com
    host: server.example.com
    type: ICMP
  - name: server2.example.com
    host: server2.example.com
    type: ICMP
  - name: server3.example.com
    host: server3.example.com
    type: ICMP
  - name: server.example.com:80
    host: server.example.com:80
    type: TCP
  - name: server2.example.com:443
    host: server2.example.com:443
    type: TCP
  - name: server3.example.com:9427
    host: server3.example.com:9427
    type: TCP
```

## Deployment

This deployment example will permit you to have as many Ping Stations as you need (LAN or WIFI) devices but at the same time decoupling the data collection from the storage and visualization.
This [docker compose](https://github.com/syepes/network_exporter/blob/master/dist/deploy/docker-compose.yml) will deploy and configure all the components plus setup Grafana with the Datasource and [Dashboard](https://github.com/syepes/network_exporter/blob/master/dist/deploy/cfg/provisioning/dashboards/network_exporter.json)

[Deployment example](https://github.com/syepes/network_exporter/blob/master/dist/deploy/)

![Deployment architecture](https://raw.githubusercontent.com/syepes/network_exporter/master/dist/deployment.jpg)

## Contribute

If you have any idea for an improvement or find a bug do not hesitate in opening an issue, just simply fork and create a pull-request to help improve the exporter.

## License

All content is distributed under the [Apache 2.0 License](http://www.apache.org/licenses/LICENSE-2.0)
Copyright &copy; 2020-2024, [Sebastian YEPES](mailto:syepes@gmail.com)
