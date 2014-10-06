package main

import (
	"bytes"
	"github.com/brnv/go-heaver"
	"github.com/op/go-logging"
	"github.com/s-kostyaev/go-cgroup"
	"github.com/s-kostyaev/go-iptables-proxy"
	"os"
	"os/exec"
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
	checkLocalNetRoute()
	go Webserver(config)
	lookup(config)
}

func lookup(config *Config) {
	hostIP := "127.0.0.1"
	for {
		enabledProxies, err := proxy.GetEnabledProxies()
		if err != nil {
			logger.Error(err.Error())
		}
		enabledProxies = proxy.FilterByComment(enabledProxies, comment)
		mappedProxies := mapProxies(enabledProxies)
		containers, err := heaver.List("local")
		if err != nil {
			logger.Error(err.Error())
		}
		for _, container := range containers {
			if container.Status != "active" {
				if prx, ok := mappedProxies[container.Ip]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						logger.Error(err.Error())
					}
					logger.Info("%s: redirect disabled",
						container.Name)
				}
				continue
			}
			limit, err := cgroup.GetParamInt("memory/lxc/"+container.Name,
				cgroup.MemoryLimit)
			if err != nil {
				limit, err = cgroup.GetParamInt("memory/lxc/"+container.Name+
					"-1", cgroup.MemoryLimit)
				if err != nil {
					logger.Error(err.Error())
					continue
				}
			}
			usage, err := cgroup.GetParamInt("memory/lxc/"+container.Name,
				cgroup.MemoryUsage)
			if err != nil {
				usage, err = cgroup.GetParamInt("memory/lxc/"+container.Name+
					"-1", cgroup.MemoryUsage)
				if err != nil {
					logger.Error(err.Error())
					continue
				}
			}
			if limit != usage {
				if prx, ok := mappedProxies[container.Ip]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						logger.Error(err.Error())
					}
					logger.Info("%s: redirect disabled", container.Name)
				}
				continue
			}
			if _, ok := mappedProxies[container.Ip]; ok {
				continue
			}
			logger.Info(
				"Memory of %s has reached the limit. ",
				container.Name)
			prx := proxy.NewProxy(container.Ip, 80,
				hostIP, config.HostPort, comment)
			err = prx.EnableForwarding()
			if err != nil {
				logger.Error(err.Error())
			}

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
}

func mapProxies(proxies []proxy.Proxy) map[string]proxy.Proxy {
	result := make(map[string]proxy.Proxy)
	for _, prx := range proxies {
		result[prx.Source.IP] = prx
	}
	return result
}

func checkLocalNetRoute() {
	cmd := exec.Command("sysctl", "-n", "net.ipv4.conf.all.route_localnet")
	cmd.Stdout = &bytes.Buffer{}
	err := cmd.Run()
	if err != nil {
		logger.Fatal(err)
	}
	if strings.Trim(cmd.Stdout.(*bytes.Buffer).String(), "\n") != "1" {
		logger.Fatal("Localnet routes disabled. You should enable it by" +
			" 'sysctl -w net.ipv4.conf.all.route_localnet=1'")
	}
}
