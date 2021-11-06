package config

import (
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/syepes/network_exporter/pkg/common"
	yaml "gopkg.in/yaml.v3"
)

// Config represents configuration for the exporter

type Targets []struct {
	Name   string   `yaml:"name" json:"name"`
	Host   string   `yaml:"host" json:"host"`
	Type   string   `yaml:"type" json:"type"`
	Proxy  string   `yaml:"proxy" json:"proxy"`
	Probe  []string `yaml:"probe" json:"probe"`
	Labels extraKV  `yaml:"labels,omitempty" json:"labels,omitempty"`
} 

type HTTPGet struct {
	Interval duration `yaml:"interval" json:"interval"`
	Timeout  duration `yaml:"timeout" json:"timeout"`
}

type TCP struct {
	Interval duration `yaml:"interval" json:"interval"`
	Timeout  duration `yaml:"timeout" json:"timeout"`
} 

type MTR struct {
	Interval duration `yaml:"interval" json:"interval"`
	Timeout  duration `yaml:"timeout" json:"timeout"`
	MaxHops  int      `yaml:"max-hops" json:"max-hops"`
	Count    int      `yaml:"count" json:"count"`
} 

type ICMP struct {
	Interval duration `yaml:"interval" json:"interval"`
	Timeout  duration `yaml:"timeout" json:"timeout"`
	Count    int      `yaml:"count" json:"count"`
} 

type Conf struct {
	Refresh    duration `yaml:"refresh" json:"refresh"`
	Nameserver string   `yaml:"nameserver" json:"nameserver"`
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
type SafeConfig struct {
	Cfg *Config
	sync.RWMutex
}

// ReloadConfig Safe configuration reload
func (sc *SafeConfig) ReloadConfig(confFile string) (err error) {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	var c = &Config{}
	f, err := os.Open(confFile)
	if err != nil {
		return fmt.Errorf("Reading config file: %s", err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	if err = decoder.Decode(c); err != nil {
		return fmt.Errorf("Parsing config file: %s", err)
	}

	// Validate and Filter config
	targets := Targets{}
	var targetNames []string

	for _, t := range c.Targets {
		if common.SrvRecordCheck(t.Host) {
			found, _ := regexp.MatchString("^ICMP|MTR|ICMP+MTR|TCP|HTTPGet$", t.Type)
			if found == false {
				return fmt.Errorf("Target '%s' has unknown check type '%s' must be one of (ICMP|MTR|ICMP+MTR|TCP|HTTPGet)", t.Name, t.Type)
			}
			
			srv_record_hosts, err := common.SrvRecordHosts(t.Host) 
			if err != nil {
				return fmt.Errorf((fmt.Sprintf("Error processing SRV target: %s", t.Host)))
			}

			for _, srv_host := range srv_record_hosts {
				targetNames = append(targetNames, srv_host)
				sub_target := t
				sub_target.Name = srv_host
				sub_target.Host = srv_host

				// Filter out the targets that are not assigned to the running host, if the `probe` is not specified don't filter
				if sub_target.Probe == nil {
					targets = append(targets, sub_target)
				} else {
					for _, p := range sub_target.Probe {
						if p == hostname {
							targets = append(targets, sub_target)
							continue
						}
					}
				}
			}
		} else {
			targetNames = append(targetNames, t.Name)
			found, _ := regexp.MatchString("^ICMP|MTR|ICMP+MTR|TCP|HTTPGet$", t.Type)
			if found == false {
				return fmt.Errorf("Target '%s' has unknown check type '%s' must be one of (ICMP|MTR|ICMP+MTR|TCP|HTTPGet)", t.Name, t.Type)
			}

			// Filter out the targets that are not assigned to the running host, if the `probe` is not specified don't filter
			if t.Probe == nil {
				targets = append(targets, t)
			} else {
				for _, p := range t.Probe {
					if p == hostname {
						targets = append(targets, t)
						continue
					}
				}
			}
		}
	}

	// Remap the filtered targets
	c.Targets = targets

	if _, err = common.HasListDuplicates(targetNames); err != nil {
		return fmt.Errorf("Parsing config file: %s", err)
	}

	// Config precheck
	if c.MTR.MaxHops < 0 || c.MTR.MaxHops > 65500 {
		return fmt.Errorf("mtr.max-hops must be between 0 and 65500")
	}
	if c.MTR.Count < 0 || c.MTR.Count > 65500 {
		return fmt.Errorf("mtr.count must be between 0 and 65500")
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
