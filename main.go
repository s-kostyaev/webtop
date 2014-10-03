package main

import (
	"github.com/op/go-logging"
	"github.com/s-kostyaev/iptables/proxy"
	"github.com/s-kostyaev/lxc"
	"net"
	"os"
	"strings"
	"time"
)

const (
	configPath   = "/etc/webtop.toml"
	comment      = "webtop"
	templatePath = "top.htm"
)

var (
	logfile   = os.Stderr
	formatter = logging.MustStringFormatter(
		"%{time:15:04:05.000000} %{pid} %{level:.8s} %{longfile} %{message}")
	loglevel = logging.INFO
	logger   = logging.MustGetLogger("webtop")
)

func initLog() {
	logging.SetBackend(logging.NewLogBackend(logfile, "", 0))
	logging.SetFormatter(formatter)
	logging.SetLevel(loglevel, logger.Module)
}

func main() {
	initLog()
	config := GetConfig(configPath)
	go Webserver(config)
	lookup(config)
}

func lookup(config *Config) {
	hostIP := getHostIP()
	for {
		enabledProxies, err := proxy.GetEnabledProxies()
		if err != nil {
			logger.Error(err.Error())
		}
		enabledProxies = proxy.FilterByComment(enabledProxies, comment)
		mappedProxies := mapProxies(enabledProxies)
		containers, err := lxc.GetContainers()
		if err != nil {
			logger.Error(err.Error())
		}
		for _, container := range containers {
			if container.State != "active" {
				if prx, ok := mappedProxies[container.IP]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						logger.Error(err.Error())
					}
					logger.Info("%s: redirect disabled",
						container.Name)
				}
				continue
			}
			limit, err := lxc.GetParamInt("memory", container.Name, "limit")
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			usage, err := lxc.GetParamInt("memory", container.Name, "usage")
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			if limit != usage {
				if prx, ok := mappedProxies[container.IP]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						logger.Error(err.Error())
					}
					logger.Info("%s: redirect disabled", container.Name)
				}
				continue
			}
			if _, ok := mappedProxies[container.IP]; ok {
				continue
			}
			logger.Info(
				"Memory of %s has reached the limit. ",
				container.Name)
			prx := proxy.NewProxy(container.IP, 80,
				hostIP, config.HostPort, comment)
			err = prx.EnableForwarding()
			if err != nil {
				logger.Error(err.Error())
			}

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
}

func getHostIP() (hostIP string) {
	hosts, err := net.InterfaceAddrs()
	if err != nil {
		logger.Fatal(err.Error())
	}
	return strings.Split(hosts[1].String(), "/")[0]
}

func mapProxies(proxies []proxy.Proxy) map[string]proxy.Proxy {
	result := make(map[string]proxy.Proxy)
	for _, prx := range proxies {
		result[prx.Source.IP] = prx
	}
	return result
}
