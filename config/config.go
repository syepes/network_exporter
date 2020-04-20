package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// Type of the test
type Type string

// Type of the test
const (
	ICMP Type = "ICMP"
	MTR  Type = "MTR"
	BOTH Type = "BOTH"
)

// Config represents configuration for the exporter
type Config struct {
	Conf struct {
		Refresh duration `yaml:"refresh"`
	} `yaml:"conf"`
	DNS struct {
		Refresh    duration `yaml:"refresh"`
		Nameserver string   `yaml:"nameserver"`
	} `yaml:"dns"`
	ICMP struct {
		Interval duration `yaml:"interval"`
		Timeout  duration `yaml:"timeout"`
		Count    int      `yaml:"count"`
	} `yaml:"icmp"`
	MTR struct {
		Interval duration `yaml:"interval"`
		Timeout  duration `yaml:"timeout"`
		MaxHops  int      `yaml:"max-hops"`
		SntSize  int      `yaml:"snt-size"`
	} `yaml:"mtr"`
	Dest []struct {
		Host  string `yaml:"host"`
		Alias string `yaml:"alias"`
		Type  string `yaml:"type"`
	} `yaml:"targets"`
}

type duration time.Duration

// SafeConfig Safe configuration reload
type SafeConfig struct {
	Cfg *Config
	sync.RWMutex
}

// ReloadConfig Safe configuration reload
func (sc *SafeConfig) ReloadConfig(confFile string) (err error) {
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

	// Validate config
	for _, t := range c.Dest {
		found, _ := regexp.MatchString("ICMP|MTR|BOTH", t.Type)
		if found == false {
			return fmt.Errorf("Unknown check type '%s' must be one of (ICMP|MTR|BOTH)", t.Type)
		}
	}

	if !strings.HasSuffix(c.DNS.Nameserver, ":53") {
		c.DNS.Nameserver += ":53"
	}

	// Config precheck
	if c.MTR.MaxHops < 0 || c.MTR.MaxHops > 65500 {
		return fmt.Errorf("mtr.max-hops must be between 0 and 65500")
	}
	if c.MTR.SntSize < 0 || c.MTR.SntSize > 65500 {
		return fmt.Errorf("mtr.snt-size must be between 0 and 65500")
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
