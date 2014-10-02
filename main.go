package main

import (
	"github.com/s-kostyaev/iptables/proxy"
	"github.com/s-kostyaev/lxc"
	"github.com/s-kostyaev/lxc/memory/monitor"
	"log"
	"net"
	"strings"
	"time"
)

const (
	ConfigPath = "/etc/webtop.toml"
	Comment    = "webtop"
)

func main() {
	config := GetConfig(ConfigPath)
	go Webserver(config)
	lookup(config)
}

func lookup(config *Config) {
	hostIP := getHostIP()
	for {
		enabledProxies, err := proxy.GetEnabledProxies()
		if err != nil {
			log.Println(err)
		}
		enabledProxies = proxy.FilterByComment(enabledProxies, Comment)
		mappedProxies := mapProxies(enabledProxies)
		containers := lxc.GetContainers()
		for _, container := range containers {
			if container.State != "active" {
				if prx, ok := mappedProxies[container.IP]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						log.Println(err)
					}
					log.Printf("%s: redirect disabled",
						container.Name)
				}
				continue
			}
			limit, err := monitor.GetInt(container.Name, "limit")
			if err != nil {
				log.Println(err)
				continue
			}
			usage, err := monitor.GetInt(container.Name, "usage")
			if err != nil {
				log.Println(err)
				continue
			}
			if limit != usage {
				if prx, ok := mappedProxies[container.IP]; ok {
					err = prx.DisableForwarding()
					if err != nil {
						log.Println(err)
					}
					log.Printf("%s: redirect disabled",
						container.Name)
				}
				continue
			}
			if _, ok := mappedProxies[container.IP]; ok {
				continue
			}
			log.Printf(
				"Memory of %s has reached the limit. ",
				container.Name)
			prx := proxy.NewProxy(container.IP, 80,
				hostIP, config.HostPort, Comment)
			err = prx.EnableForwarding()
			if err != nil {
				log.Println(err)
			}

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
}

func getHostIP() (hostIP string) {
	hosts, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal(err)
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
