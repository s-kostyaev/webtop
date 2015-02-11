package main

import (
	"bytes"
	"fmt"
	"github.com/brnv/go-heaver"
	"github.com/s-kostyaev/go-iptables-proxy"
	"github.com/s-kostyaev/go-linux-net-bridge"
	"github.com/s-kostyaev/go-lxc"
	"net"
	"os/exec"
	"strings"
	"time"
)

type freezedContainer struct {
	*lxc.Container
	Proxy         *proxy.Proxy
	VirtualIp     string
	StopWebserver chan bool
}

var freezedContainers = make(map[string]freezedContainer)

func main() {
	setupLogger()
	config := getConfig(configPath)
	checkLocalNetRoute()
	hostIp := mustGetHostIp()
	err := prepareVirtualNet(config.VirtualIpSubnet, hostIp)
	if err != nil {
		logger.Error(err.Error())
	}
	garbageClear()
	lookup(config)
}

func lookup(config *Config) {
	for {
		containers, err := heaver.List("local")
		if err != nil {
			logger.Error(err.Error())
		}
		for _, container := range containers {
			if container.Status != "active" {
				currentContainer, ok := freezedContainers[container.Name]
				if ok {
					currentContainer.Unfreeze()
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
				currentContainer, ok := freezedContainers[container.Name]
				if ok {
					currentContainer.Unfreeze()
				}
				continue
			}
			if _, ok := freezedContainers[container.Name]; ok {
				continue
			}
			logger.Info("Memory of %s has reached the limit. ", container.Name)
			containerIsFreezed(&container, config)

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
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

func checkLocalNetRoute() {
	cmd := exec.Command("sysctl", "-n", "net.ipv4.conf.all.route_localnet")
	cmd.Stdout = &bytes.Buffer{}
	err := cmd.Run()
	if err != nil {
		logger.Fatal(err.Error())
	}
	if strings.Trim(cmd.Stdout.(*bytes.Buffer).String(), "\n") != "1" {
		logger.Fatal("Localnet routes disabled (RFC 4213: Packets received " +
			"on an interface with a loopback destination address must be " +
			"dropped). But this service should may enable redirect from " +
			"container to host. You should enable localnet routes by" +
			" 'sysctl -w net.ipv4.conf.all.route_localnet=1'")
	}

}

func prepareVirtualNet(virtualSubnet, hostIp string) error {

	if !bridge.IsBridgeExist(bridgeName) {
		err := bridge.CreateBridge(bridgeName)
		if err != nil {
			return err
		}
	}

	err := bridge.StartBridge(bridgeName)
	if err != nil {
		return err
	}

	return setVirtualSubnetRoutes(virtualSubnet, hostIp)
}

func setVirtualSubnetRoutes(virtualSubnet, hostIp string) error {
	cmd := exec.Command("ip", "route", "add", virtualSubnet, "via", hostIp)
	return cmd.Run()
}

func containerToVirtualIp(containerIp, virtualSubnet string) (string, error) {
	tmp := strings.Split(virtualSubnet, "/")
	netMask := tmp[1]
	splittedVirtualIp := strings.Split(tmp[0], ".")
	splittedContainerIp := strings.Split(containerIp, ".")
	resultIp := ""
	switch netMask {
	case "24":
		resultIp = fmt.Sprintf("%s.%s.%s.%s",
			splittedVirtualIp[0], splittedVirtualIp[1],
			splittedVirtualIp[2], splittedContainerIp[3])
	case "16":
		resultIp = fmt.Sprintf("%s.%s.%s.%s",
			splittedVirtualIp[0], splittedVirtualIp[1],
			splittedContainerIp[2], splittedContainerIp[3])
	case "8":
		resultIp = fmt.Sprintf("%s.%s.%s.%s",
			splittedVirtualIp[0], splittedContainerIp[1],
			splittedContainerIp[2], splittedContainerIp[3])
	default:
		return "", fmt.Errorf("Bad netmask in config file. Only /8, /16 and " +
			"/24 netmasks are accepted.")
	}
	return resultIp, nil
}

func newFreezedContainer(container *lxc.Container, virtualSubnet string,
) freezedContainer {
	result := freezedContainer{}
	result.Container = container
	virtualIp, err := containerToVirtualIp(container.Ip, virtualSubnet)
	if err != nil {
		logger.Error(err.Error())
	}
	result.VirtualIp = virtualIp
	result.Proxy = proxy.NewProxy(container.Ip, 80, virtualIp, 80, comment,
		true)

	return result
}

func (container *freezedContainer) Unfreeze() {
	container.StopWebserver <- true
	container.Proxy.DisableForwarding()
	bridge.RemoveIpFromBridge(container.VirtualIp, bridgeName)

	delete(freezedContainers, container.Name)
}

func containerIsFreezed(container *lxc.Container, config *Config) {
	if _, ok := freezedContainers[container.Name]; ok {
		return
	}

	newContainer := newFreezedContainer(container, config.VirtualIpSubnet)
	bridge.AssignIpToBridge(newContainer.VirtualIp, bridgeName)
	err := newContainer.Proxy.EnableForwarding()
	if err != nil {
		logger.Error(err.Error())
	}
	newContainer.StopWebserver = startWebserver(newContainer.VirtualIp, 80)

	freezedContainers[container.Name] = newContainer
}

func garbageClear() {
	enabledProxies, err := proxy.GetEnabledProxies()
	if err != nil {
		logger.Error(err.Error())
	}
	enabledProxies = proxy.FilterByComment(enabledProxies, comment)
	for _, currentProxy := range enabledProxies {
		bridge.RemoveIpFromBridge(currentProxy.Dest.IP, bridgeName)
		currentProxy.DisableForwarding()
	}
}
