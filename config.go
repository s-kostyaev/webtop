package main

import (
	"github.com/BurntSushi/toml"
	"github.com/op/go-logging"
	"io/ioutil"
	"os"
	"time"
)

const (
	configPath = "/etc/webtop.toml"
	comment    = "webtop"
)

var (
	logfile   = os.Stderr
	formatter = logging.MustStringFormatter(
		"%{time:15:04:05.000000} %{pid} %{level:.8s} %{longfile} %{message}")
	loglevel = logging.INFO
	logger   = logging.MustGetLogger("webtop")
)

type Config struct {
	LookupTimeout duration
}

type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func setupLogger() {
	logging.SetBackend(logging.NewLogBackend(logfile, "", 0))
	logging.SetFormatter(formatter)
	logging.SetLevel(loglevel, logger.Module)
}

func getConfig(configPath string) *Config {
	buf, err := ioutil.ReadFile(configPath)
	if err != nil {
		logger.Fatal(err.Error())
	}
	config := Config{}
	_, err = toml.Decode(string(buf), &config)
	if err != nil {
		logger.Fatal(err.Error())
	}
	return &config
}
