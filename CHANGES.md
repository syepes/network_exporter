# Changes to Support Multiple Hosts per Target

## Summary
This update adds support for specifying multiple hosts per target in the network exporter configuration. The changes maintain backward compatibility while allowing users to define multiple hosts for a single target.

## Files Modified

### 1. `/workspace/config/config.go`
- Updated the `Targets` struct to include both `Host` (single host) and `Hosts` (multiple hosts) fields
- Modified the `ReloadConfig` function to handle both single host and multiple hosts configurations
- Added logic to process each host in the `hosts` array as a separate target
- Maintained all existing functionality for SRV records, filtering, etc.

### 2. `/workspace/network_exporter.yml`
- Updated the example configuration to demonstrate the new `hosts` field
- Changed the `cloudflare-dns` target to use the new format with multiple IP addresses
- Included IPv6 example in the hosts list

## Configuration Examples

### Old Format (Still Supported)
```yaml
targets:
  - name: cloudflare-dns
    host: 1.1.1.1
    type: ICMP+MTR
```

### New Format (Multiple Hosts)
```yaml
targets:
  - name: cloudflare-dns
    hosts:
      - 1.1.1.1
      - 1.0.0.1
      - 2606:4700:4700::1111  # IPv6 example
    type: ICMP+MTR
```

## Implementation Details
- When both `host` and `hosts` are specified, `hosts` takes precedence
- If no host is specified, an error is logged and the target is skipped
- Each host in the `hosts` array is processed as a separate target with the same configuration
- All existing functionality (SRV records, probe filtering, etc.) continues to work as before