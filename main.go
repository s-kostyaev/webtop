package main

import (
	"encoding/json"
	"fmt"
	"github.com/brnv/go-heaver"
	"github.com/s-kostyaev/go-lxc"
	"github.com/s-kostyaev/webtop-protocol"
	"github.com/shirou/gopsutil"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

var sharedConnection net.Conn
var webtopContainerName string

func main() {
	setupLogger()
	config := getConfig(configPath)
	prepareEnvironment()
	go commandManager()
	lookup(config)
}

func lookup(config *Config) {
	for {
		containers, err := heaver.List("local")
		if err != nil {
			logger.Error(err.Error())
		}
		freezedIps := getAssignedIps(webtopContainerName)
		for _, container := range containers {
			if container.Status != "active" {
				if _, ok := freezedIps[container.Ip]; ok {
					containerUnfreeze(container)
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
				if _, ok := freezedIps[container.Ip]; ok {
					containerUnfreeze(container)
				}
				continue
			}
			if _, ok := freezedIps[container.Ip]; ok {
				continue
			}
			logger.Info("Memory of %s has reached the limit. ", container.Name)
			containerIsFreezed(container)

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
}

func containerUnfreeze(container lxc.Container) {
	err := deleteIp(container.Ip, webtopContainerName)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	err = assignIp(container.Ip, container.Name)
	if err != nil {
		logger.Error(err.Error())
	}
}

func containerIsFreezed(container lxc.Container) {
	err := deleteIp(container.Ip, container.Name)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	err = assignIp(container.Ip, webtopContainerName)
	if err != nil {
		logger.Error(err.Error())
	}
}

func prepareEnvironment() {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Panic(err.Error())
	}
	webtopContainerName = "webtop-" + hostname

	containerList, err := heaver.List(hostname)
	if err != nil {
		logger.Panic(err.Error())
	}

	// if container not found
	if _, ok := containerList[webtopContainerName]; !ok {
		// create container
		imageName := []string{"abox"}
		_, err = heaver.Create(webtopContainerName, imageName)
		if err != nil {
			logger.Panic(err.Error())
		}
		// TODO: install webserver

		// TODO: enable and start webserver unit

	}
	rootfs, err := lxc.GetRootFS(webtopContainerName)
	if err != nil {
		logger.Panic(err.Error())
	}
	socketPath := rootfs + "/webtop.sock"
	go listenSocket(socketPath)
}

func listenSocket(path string) {
	// remove socket file if exist
	os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		logger.Error("Listen error: %s\n", err.Error())
		go listenSocket(path)
		return
	}
	connection, err := listener.Accept()
	if err != nil {
		logger.Error("Access error: %s\n", err.Error())
		go listenSocket(path)
		return
	}
	sharedConnection = connection
}

func commandManager() {
	decoder := json.NewDecoder(sharedConnection)
	for {
		request := protocol.Request{}
		err := decoder.Decode(&request)
		if err != nil {
			logger.Error(err.Error())
			go commandManager()
			return
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
		process, err := gopsutil.NewProcess(int32(pid))
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
	encoder := json.NewEncoder(sharedConnection)
	err := encoder.Encode(answer)
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

func deleteIp(ip, containerName string) error {
	cmd := exec.Command("lxc-attach", "-n", containerName, "-e", "--", "/bin/ip",
		"addr", "del", ip, "dev", "eth0")
	return cmd.Run()
}

func assignIp(ip, containerName string) error {
	cmd := exec.Command("lxc-attach", "-n", containerName, "-e", "--", "/bin/ip",
		"addr", "add", ip, "dev", "eth0")
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
		if tmp[1] != "lo" {
			result[tmp[3]] = struct{}{}
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
		process, err := gopsutil.NewProcess(pid)
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
