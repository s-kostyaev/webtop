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
	ConfigPath   = "/etc/webtop.toml"
	Comment      = "webtop"
	TemplatePath = "top.htm"
)

var (
	Logfile   = os.Stderr
	Formatter = logging.MustStringFormatter(
		"%{time:15:04:05.000000} %{pid} %{level:.8s}" +
			" %{longfile} %{message}")
	Loglevel = logging.INFO
	Log      = logging.MustGetLogger("webtop")
)

func initLog() {
	logging.SetBackend(logging.NewLogBackend(Logfile, "", 0))
	logging.SetFormatter(Formatter)
	logging.SetLevel(Loglevel, Log.Module)
}

func main() {
	initLog()
	config := GetConfig(ConfigPath)
	go Webserver(config)
	lookup(config)
}

func lookup(config *Config) {
	hostIP := getHostIP()
	for {
		enabledProxies, err := proxy.GetEnabledProxies()
		if err != nil {
			Log.Error(err.Error())
		}
		enabledProxies = proxy.FilterByComment(enabledProxies, Comment)
		mappedProxies := mapProxies(enabledProxies)
		containers, err := lxc.GetContainers()
		if err != nil {
			Log.Error(err.Error())
		}
		for _, container := range containers {
			if container.State != "active" {
				if prx, ok := mappedProxies[container.IP]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						Log.Error(err.Error())
					}
					Log.Info("%s: redirect disabled",
						container.Name)
				}
				continue
			}
			limit, err := lxc.GetParamInt("memory", container.Name,
				"limit")
			if err != nil {
				Log.Error(err.Error())
				continue
			}
			usage, err := lxc.GetParamInt("memory", container.Name,
				"usage")
			if err != nil {
				Log.Error(err.Error())
				continue
			}
			if limit != usage {
				if prx, ok := mappedProxies[container.IP]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						Log.Error(err.Error())
					}
					Log.Info("%s: redirect disabled",
						container.Name)
				}
				continue
			}
			if _, ok := mappedProxies[container.IP]; ok {
				continue
			}
			Log.Info(
				"Memory of %s has reached the limit. ",
				container.Name)
			prx := proxy.NewProxy(container.IP, 80,
				hostIP, config.HostPort, Comment)
			err = prx.EnableForwarding()
			if err != nil {
				Log.Error(err.Error())
			}

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
}

func getHostIP() (hostIP string) {
	hosts, err := net.InterfaceAddrs()
	if err != nil {
		Log.Fatal(err.Error())
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
