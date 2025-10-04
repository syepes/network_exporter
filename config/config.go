package config

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creasty/defaults"
	"github.com/syepes/network_exporter/pkg/common"

	yaml "gopkg.in/yaml.v3"
)

// Config represents configuration for the exporter

type Targets []struct {
	Name     string   `yaml:"name" json:"name"`
	Host     string   `yaml:"host" json:"host"`
	Type     string   `yaml:"type" json:"type"`
	Proxy    string   `yaml:"proxy" json:"proxy"`
	Probe    []string `yaml:"probe" json:"probe"`
	SourceIp string   `yaml:"source_ip" json:"source_ip"`
	Labels   extraKV  `yaml:"labels,omitempty" json:"labels,omitempty"`
}

type HTTPGet struct {
	Interval duration `yaml:"interval" json:"interval" default:"15s"`
	Timeout  duration `yaml:"timeout" json:"timeout" default:"14s"`
}

type TCP struct {
	Interval duration `yaml:"interval" json:"interval" default:"5s"`
	Timeout  duration `yaml:"timeout" json:"timeout" default:"4s"`
}

type MTR struct {
	Interval    duration `yaml:"interval" json:"interval" default:"5s"`
	Timeout     duration `yaml:"timeout" json:"timeout" default:"4s"`
	MaxHops     int      `yaml:"max-hops" json:"max-hops" default:"30"`
	Count       int      `yaml:"count" json:"count" default:"10"`
	PayloadSize int      `yaml:"payload_size" json:"payload_size" default:"56"`
	Protocol    string   `yaml:"protocol" json:"protocol" default:"icmp"`
	TcpPort     string   `yaml:"tcp_port" json:"tcp_port" default:"80"`
}

type ICMP struct {
	Interval    duration `yaml:"interval" json:"interval" default:"5s"`
	Timeout     duration `yaml:"timeout" json:"timeout" default:"4s"`
	Count       int      `yaml:"count" json:"count" default:"10"`
	PayloadSize int      `yaml:"payload_size" json:"payload_size" default:"56"`
}

type Conf struct {
	Refresh           duration `yaml:"refresh" json:"refresh" default:"0s"`
	Nameserver        string   `yaml:"nameserver" json:"nameserver"`
	NameserverTimeout duration `yaml:"nameserver_timeout" json:"nameserver_timeout" default:"250ms"`
}

type Config struct {
	Conf    `yaml:"conf" json:"conf"`
	ICMP    `yaml:"icmp" json:"icmp"`
	MTR     `yaml:"mtr" json:"mtr"`
	TCP     `yaml:"tcp" json:"tcp"`
	HTTPGet `yaml:"http_get" json:"http_get"`
	Targets `yaml:"targets" json:"targets"`
}

type duration time.Duration

type extraKV struct {
	Kv map[string]string `yaml:"kv,omitempty" json:"kv,omitempty"`
}

// UnmarshalYAML is used to unmarshal into map[string]string
func (b *extraKV) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return unmarshal(&b.Kv)
}

// SafeConfig Safe configuration reload
type Resolver struct {
	Resolver *net.Resolver
	Timeout  time.Duration
}

// SafeConfig Safe configuration reload
type SafeConfig struct {
	Cfg *Config
	sync.RWMutex
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func parse(data []byte, c *Config) error {
	err := yaml.Unmarshal(data, &c)

	if err != nil {
		return fmt.Errorf("unmarshaling config: %s", err)
	}

	return nil
}

// ReloadConfig Safe configuration reload
func (sc *SafeConfig) ReloadConfig(logger *slog.Logger, confFile string, confFileHeaders http.Header) (err error) {
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %s", err)
	}

	var data []byte

	if isHTTPURL(confFile) {
		logger.Debug("Loading config from HTTP")

		req, err := http.NewRequest("GET", confFile, nil)
		if err != nil {
			return fmt.Errorf("creating request: %s", err)
		}

		for key, values := range confFileHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		// Configure transport with connection pooling for better scalability
		transport := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     0,
		}
		client := &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("fetching config file: %s", err)
		}
		defer resp.Body.Close()

		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading config file: %s", err)
		}
	} else {
		logger.Debug("Loading config from file")

		f, err := os.Open(confFile)
		if err != nil {
			return fmt.Errorf("reading config file: %s", err)
		}
		defer f.Close()

		data, err = io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("reading config file: %s", err)
		}
	}

	var c = &Config{}

	err = parse(data, c)
	if err != nil {
		return fmt.Errorf("parsing config file: %s", err)
	}

	if err := defaults.Set(c); err != nil {
		return fmt.Errorf("setting defaults: %s", err)
	}

	// Validate and Filter config
	targets := Targets{}
	re := regexp.MustCompile("^ICMP|MTR|ICMP+MTR|TCP|HTTPGet$")
	for _, t := range c.Targets {
		if common.SrvRecordCheck(t.Host) {
			found := re.MatchString(t.Type)
			if !found {
				logger.Error("Unknown check type", "type", "Config", "func", "ReloadConfig", "target", t.Name, "check_type", t.Type, "allowed", "(ICMP|MTR|ICMP+MTR|TCP|HTTPGet)")
				continue
			}
			// Check that SRV record's type is TCP, if config's type is TCP
			if t.Type == "TCP" {
				if !strings.EqualFold(t.Type, strings.Split(t.Host, ".")[1][1:]) {
					logger.Error("Target type doesn't match SRV record protocol", "type", "Config", "func", "ReloadConfig", "target", t.Name, "check_type", t.Type, "srv_proto", strings.Split(t.Host, ".")[1][1:])
					continue
				}
			}

			srv_record_hosts, err := common.SrvRecordHosts(t.Host)
			if err != nil {
				logger.Error("Error processing SRV record", "type", "Config", "func", "ReloadConfig", "target", t.Host, "err", err)
				continue
			}

			for _, srvTarget := range srv_record_hosts {
				sub_target := t
				sub_target.Name = srvTarget
				sub_target.Host = srvTarget

				// Filter out the targets that are not assigned to the running host, if the `probe` is not specified don't filter
				if sub_target.Probe == nil {
					targets = append(targets, sub_target)
				} else {
					for _, p := range sub_target.Probe {
						if p == hostname {
							targets = append(targets, sub_target)
							break
						}
					}
				}
			}
		} else {
			found := re.MatchString(t.Type)
			if !found {
				logger.Error("Unknown check type", "type", "Config", "func", "ReloadConfig", "target", t.Name, "check_type", t.Type, "allowed", "(ICMP|MTR|ICMP+MTR|TCP|HTTPGet)")
				continue
			}

			// Filter out the targets that are not assigned to the running host, if the `probe` is not specified don't filter
			if t.Probe == nil {
				targets = append(targets, t)
			} else {
				for _, p := range t.Probe {
					if p == hostname {
						targets = append(targets, t)
						break
					}
				}
			}
		}
	}

	// Remap the filtered targets
	c.Targets = targets

	if _, err = HasDuplicateTargets(c.Targets); err != nil {
		return fmt.Errorf("parsing config file: %s", err)
	}

	// Config precheck
	if c.ICMP.Interval <= 0 || c.MTR.Interval <= 0 || c.TCP.Interval <= 0 || c.HTTPGet.Interval <= 0 {
		return fmt.Errorf("intervals (icmp,mtr,tcp,http_get) must be >0")
	}
	if c.MTR.MaxHops < 0 || c.MTR.MaxHops > 65500 {
		return fmt.Errorf("mtr.max-hops must be between 0 and 65500")
	}
	if c.MTR.Count < 0 || c.MTR.Count > 65500 {
		return fmt.Errorf("mtr.count must be between 0 and 65500")
	}
	if c.MTR.Protocol != "icmp" && c.MTR.Protocol != "tcp" {
		return fmt.Errorf("mtr.protocol must be 'icmp' or 'tcp'")
	}

	sc.Lock()
	sc.Cfg = c
	sc.Unlock()

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (d *duration) UnmarshalYAML(unmashal func(interface{}) error) error {
	var s string
	if err := unmashal(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = duration(dur)
	return nil
}

// Duration is a convenience getter.
func (d duration) Duration() time.Duration {
	return time.Duration(d)
}

// Set updates the underlying duration.
func (d *duration) Set(dur time.Duration) {
	*d = duration(dur)
}

// HasDuplicateTargets Find duplicates with same type
func HasDuplicateTargets(m Targets) (bool, error) {
	tmp := map[string]map[string]bool{
		"TCP":     make(map[string]bool),
		"ICMP":    make(map[string]bool),
		"MTR":     make(map[string]bool),
		"HTTPGet": make(map[string]bool),
	}

	for _, t := range m {
		if t.Type == "ICMP+MTR" {
			if tmp["MTR"][t.Name] {
				return true, fmt.Errorf("found duplicated record: %s", t.Name)
			}
			tmp["MTR"][t.Name] = true
			if tmp["ICMP"][t.Name] {
				return true, fmt.Errorf("found duplicated record: %s", t.Name)
			}
			tmp["ICMP"][t.Name] = true
		} else {
			if tmp[t.Type][t.Name] {
				return true, fmt.Errorf("found duplicated record: %s", t.Name)
			}
			tmp[t.Type][t.Name] = true
		}
	}
	return false, nil
}
