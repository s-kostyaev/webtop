package main

import (
	"encoding/json"
	"fmt"
	"github.com/brnv/go-heaver"
	"github.com/s-kostyaev/go-lxc"
	"github.com/s-kostyaev/webtop-protocol"
	"github.com/shirou/gopsutil/process"
	"net"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

var webtopContainer string
var webtopContainerRootFs string

func main() {
	setupLogger()
	config := getConfig(configPath)
	prepareEnvironment()
	go commandManager()
	lookup(config)
}

func lookup(config *Config) {
	for {
		containers, err := lxc.GetContainers()
		if err != nil {
			logger.Error(err.Error())
			continue
		}
		freezedIps := getAssignedIps(webtopContainer)
		for _, container := range containers {
			if container == webtopContainer {
				continue
			}
			containerIp, err := lxc.GetConfigItem(container, "lxc.network.ipv4")
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			state, err := lxc.GetState(container)
			if state != "RUNNING" {
				if _, ok := freezedIps[containerIp]; ok {
					logger.Info("Container %s is %s", container,
						strings.ToLower(state))
					deleteIp(containerIp, webtopContainer)
				}
				continue
			}
			limit, err := lxc.GetMemoryLimit(container)
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			usage, err := lxc.GetMemoryUsage(container)
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			if limit != usage {
				if _, ok := freezedIps[containerIp]; ok {
					logger.Info("Memory of %s is now free", container)
					containerUnfreeze(container)
				}
				continue
			}
			if _, ok := freezedIps[containerIp]; ok {
				continue
			}
			logger.Info("Memory of %s has reached the limit. ", container)
			containerIsFreezed(container)

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
}

func containerUnfreeze(container string) {
	logger.Info("Unfreezing %s", container)
	containerIp, err := lxc.GetConfigItem(container, "lxc.network.ipv4")
	if err != nil {
		logger.Error(err.Error())
		return
	}
	containerGw, err := lxc.GetConfigItem(container, "lxc.network.ipv4.gateway")
	if err != nil {
		logger.Error(err.Error())
		return
	}
	err = deleteIp(containerIp, webtopContainer)
	for err != nil {
		logger.Error(err.Error())
		err = deleteIp(containerIp, webtopContainer)
	}
	err = assignIp(containerIp, container)
	for err != nil {
		logger.Error(err.Error())
		err = assignIp(containerIp, container)
	}
	setDefaultGateway(container, containerGw)
	for err != nil {
		logger.Error(err.Error())
		err = setDefaultGateway(container, containerGw)
	}
}

func containerIsFreezed(container string) {
	containerIp, err := lxc.GetConfigItem(container, "lxc.network.ipv4")
	if err != nil {
		logger.Error(err.Error())
		return
	}
	err = deleteIp(containerIp, container)
	for err != nil {
		logger.Error(err.Error())
		err = deleteIp(containerIp, container)
	}
	err = assignIp(containerIp, webtopContainer)
	for err != nil {
		logger.Error(err.Error())
		err = assignIp(containerIp, webtopContainer)
	}
}

func prepareEnvironment() {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Panic(err.Error())
	}
	webtopContainer = "webtop-" + hostname

	containerList, err := heaver.List(hostname)
	if err != nil {
		logger.Panic(err.Error())
	}

	// if container not found
	if _, ok := containerList[webtopContainer]; !ok {
		// create container
		imageName := []string{"abox"}
		_, err = heaver.Create(webtopContainer, imageName)
		if err != nil {
			logger.Panic(err.Error())
		}
		// TODO: install webserver

		// TODO: enable and start webserver unit

	}
	webtopContainerRootFs, err = lxc.GetRootFS(webtopContainer)
	if err != nil {
		logger.Panic(err.Error())
	}
}

func commandManager() {
	commandSocketPath := webtopContainerRootFs + "/webtop-command.sock"
	// remove socket file if exist
	os.Remove(commandSocketPath)
	listener, err := net.Listen("unix", commandSocketPath)
	if err != nil {
		logger.Error("Listen error: %s\n", err.Error())
		go commandManager()
		return
	}
	connection, err := listener.Accept()
	if err != nil {
		logger.Error("Access error: %s\n", err.Error())
		go commandManager()
		return
	}
	decoder := json.NewDecoder(connection)
	for {
		request := protocol.Request{}
		err := decoder.Decode(&request)
		if err != nil {
			if err.Error() != "EOF" {
				logger.Error(err.Error())
			}
			go commandManager()
			return
		}
		if reflect.DeepEqual(request, protocol.Request{}) {
			continue
		}
		go processRequest(request)
	}
}

func processRequest(request protocol.Request) {
	answer := protocol.Answer{}
	answer.Id = request.Id
	answer.Status = "ok"
	cmd := strings.Split(request.Cmd, "/")
	switch cmd[0] {
	case "top":
		processTop(&answer, request.Host)
	case "cleartmp":
		if err := lxc.ClearTmp(cmd[1]); err != nil {
			logger.Error(err.Error())
			answer.Status = "Error"
			answer.Error = err.Error()
		} else {
			processTop(&answer, request.Host)
		}
	case "kill":
		pid, _ := strconv.Atoi(cmd[1])
		process, err := process.NewProcess(int32(pid))
		if err != nil {
			logger.Error(err.Error())
			answer.Status = "Error"
			answer.Error = err.Error()
		} else {
			process.Kill()
			processTop(&answer, request.Host)
		}
	default:
		answer.Status = "Error"
		answer.Error = fmt.Sprintf("Command %s does not implemented", cmd[0])
	}
	dataSocketPath := webtopContainerRootFs + "/webtop-data.sock"
	connection, err := net.Dial("unix", dataSocketPath)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	defer connection.Close()

	encoder := json.NewEncoder(connection)
	err = encoder.Encode(answer)
	for err != nil {
		logger.Error(err.Error())
		err = encoder.Encode(answer)
	}
}

func processTop(answer *protocol.Answer, host string) {
	containerIPs, err := net.LookupIP(host)
	if err != nil {
		logger.Error(err.Error())
		answer.Status = "Error"
		answer.Error = err.Error()
		return
	}
	answer.Data = getContainerTopByIp(fmt.Sprint(containerIPs[0]))
}

func deleteIp(ip, container string) error {
	cmd := exec.Command("lxc-attach", "-n", container, "-e", "--", "/bin/ip",
		"addr", "del", ip, "dev", "eth0")
	return cmd.Run()
}

func assignIp(ip, container string) error {
	logger.Debug("%s assigning to %s", ip, container)
	cmd := exec.Command("lxc-attach", "-n", container, "-e", "--", "/bin/ip",
		"addr", "add", ip, "dev", "eth0")
	return cmd.Run()
}

func setDefaultGateway(container, gateway string) error {
	cmd := exec.Command("lxc-attach", "-e", "-n", container, "--",
		"/sbin/route", "add", "default", "gw", gateway, "dev", "eth0")
	return cmd.Run()
}

func getAssignedIps(containerName string) map[string]struct{} {
	result := make(map[string]struct{})
	cmd := exec.Command("lxc-attach", "-n", containerName, "-e", "--", "/bin/ip",
		"-4", "-o", "addr")
	out, err := cmd.Output()
	if err != nil {
		logger.Error(err.Error())
	}
	splittedOut := strings.Split(string(out), "\n")
	for _, ipString := range splittedOut {
		tmp := strings.Fields(ipString)
		if len(tmp) > 0 {
			if tmp[1] != "lo" {
				result[tmp[3]] = struct{}{}
			}
		}
	}

	return result
}

func NewContainerTop(name string, limit int) protocol.ContainerTop {
	ct := protocol.ContainerTop{}
	ct.Name = name
	ct.LimitMb = limit / 1024 / 1024
	tmpfs, err := lxc.IsTmpTmpfs(name)
	if err != nil {
		logger.Error(err.Error())
	}
	if tmpfs {
		tmpUsage, err := lxc.GetTmpUsageMb(name)
		if err != nil {
			logger.Error(err.Error())
		}
		ct.TmpUsage = tmpUsage
	}
	ct.Procs = top(name)
	return ct
}

func getContainerTopByIp(ip string) protocol.ContainerTop {
	result := protocol.ContainerTop{}
	containers, err := heaver.List("local")
	if err != nil {
		logger.Error(err.Error())
	}
	for _, container := range containers {
		if container.Ip == ip {
			limit, err := lxc.GetMemoryLimit(container.Name)
			if err != nil {
				logger.Error(err.Error())
			}
			result = NewContainerTop(container.Name, limit)
			break
		}
	}
	if result.Name == "" {
		logger.Error("Cannot associate resolved IP to container")
	}
	return result
}

func top(container string) protocol.ByMemory {
	res := protocol.ByMemory{}

	pids, err := lxc.GetMemoryPids(container)
	if err != nil {
		logger.Panic(err.Error())
	}

	for _, pid := range pids {
		pr := protocol.Proc{}
		pr.Pid = fmt.Sprint(pid)
		process, err := process.NewProcess(pid)
		if err != nil {
			logger.Panic(err.Error())
		}
		cmd, err := process.Cmdline()
		if err != nil {
			logger.Panic(err.Error())
		}
		pr.Command = cmd
		mem, err := process.MemoryInfo()
		if err != nil {
			logger.Panic(err.Error())
		}
		pr.Memory = fmt.Sprint(mem.RSS / 1024 / 1024)
		res = append(res, pr)
	}
	sort.Sort(sort.Reverse(res))
	return res
}
