package main

import (
	"bytes"
	"github.com/brnv/go-heaver"
	"github.com/s-kostyaev/go-iptables-proxy"
	"github.com/s-kostyaev/go-lxc"
	"os/exec"
	"strings"
	"time"
)

func main() {
	setupLogger()
	config := getConfig(configPath)
	checkLocalNetRoute()
	go Webserver(config)
	lookup(config)
}

func lookup(config *Config) {
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
					if err = prx.DisableForwarding(); err != nil {
						logger.Error(err.Error())
					}
					logger.Info("%s: redirect disabled", container.Name)
				}
				continue
			}
			limit, err := lxc.GetMemoryLimit(container.Name)
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			usage, err := lxc.GetMemoryUsage(container.Name)
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			if limit != usage {
				if prx, ok := mappedProxies[container.Ip]; ok {
					if err = prx.DisableForwarding(); err != nil {
						logger.Error(err.Error())
					}
					logger.Info("%s: redirect disabled", container.Name)
				}
				continue
			}
			if _, ok := mappedProxies[container.Ip]; ok {
				continue
			}
			logger.Info("Memory of %s has reached the limit. ", container.Name)
			prx := proxy.NewProxy(container.Ip, 80, "127.0.0.1",
				config.HostPort, comment)
			if err = prx.EnableForwarding(); err != nil {
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
		logger.Fatal("Localnet routes disabled (RFC 4213: Packets received " +
			"on an interface with a loopback destination address must be " +
			"dropped). But this service should may enable redirect from " +
			"container to host. You should enable localnet routes by" +
			" 'sysctl -w net.ipv4.conf.all.route_localnet=1'")
	}
}
