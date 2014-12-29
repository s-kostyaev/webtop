package main

import (
	"github.com/brnv/go-heaver"
	"github.com/s-kostyaev/go-iptables-proxy"
	"github.com/s-kostyaev/go-lxc"
	"net"
	"time"
)

func main() {
	setupLogger()
	config := getConfig(configPath)
	go Webserver(config)
	lookup(config)
}

func lookup(config *Config) {
	hostIp := mustGetHostIp()
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
			prx := proxy.NewProxy(container.Ip, 80, hostIp,
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

// return host ip address string
// or terminate program
func mustGetHostIp() string {

	addrs, err := net.InterfaceAddrs()

	if err != nil {
		logger.Fatal(err.Error())
	}

	for _, address := range addrs {

		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}

		}
	}

	logger.Fatal("Could not get host ip address")
	return ""
}
