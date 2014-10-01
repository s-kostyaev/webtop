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
)

func main() {
	config := GetConfig(ConfigPath)
	go Webserver(config)
	lookup(config)
}

func filterProxies(proxies []proxy.Proxy,
	hostIP string, hostPort int) []proxy.Proxy {
	var filProxies []proxy.Proxy
	for _, prx := range proxies {
		if prx.Dest.IP == hostIP &&
			prx.Dest.Port == hostPort {
			filProxies = append(filProxies, prx)
		}
	}
	return filProxies
}

func lookup(config *Config) {
	hostIP := getHostIP()
	for {
		enabledProxies, err := proxy.GetEnabledProxies()
		if err != nil {
			log.Println(err)
		}
		enabledProxies = filterProxies(enabledProxies,
			hostIP, config.HostPort)
		containers := lxc.GetContainers()
		for _, container := range containers {
			if container.State != "active" {
				for _, prx := range enabledProxies {
					if prx.Source.IP == container.IP {
						err = prx.DisableRedirect()
						if err != nil {
							log.Println(err)
						}
						break
					}
				}
				continue
			}
			limit, err := monitor.Get(container.Name, "limit")
			if err != nil {
				log.Println(err)
				continue
			}
			usage, err := monitor.Get(container.Name, "usage")
			if err != nil {
				log.Println(err)
				continue
			}
			if limit != usage {
				for _, prx := range enabledProxies {
					if prx.Source.IP == container.IP {
						log.Printf(
							"%s: redirect disabled",
							container.Name)
						err = prx.DisableRedirect()
						if err != nil {
							log.Println(err)
						}
						break
					}
				}
				continue
			}
			log.Printf(
				"Memory of %s has reached the limit. ",
				container.Name)
			prx := proxy.NewProxy(container.IP, 80,
				hostIP, config.HostPort)
			err = prx.EnableRedirect()
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
