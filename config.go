package main

import (
	"github.com/BurntSushi/toml"
	"github.com/op/go-logging"
	"html/template"
	"io/ioutil"
	"os"
)

const (
	configPath   = "/etc/webtop.toml"
	comment      = "webtop"
	templatePath = "/usr/share/webtop/top.htm"
)

var (
	logfile   = os.Stderr
	formatter = logging.MustStringFormatter(
		"%{time:15:04:05.000000} %{pid} %{level:.8s} %{longfile} %{message}")
	loglevel = logging.INFO
	logger   = logging.MustGetLogger("webtop")
	tem      = template.Must(template.ParseFiles(templatePath))
)

type Config struct {
	LookupTimeout duration
	HostPort      int
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
