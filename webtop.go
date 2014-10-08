package main

import (
	"fmt"
	"github.com/brnv/go-heaver"
	"github.com/s-kostyaev/go-lxc"
	"github.com/shirou/gopsutil"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type duration struct {
	time.Duration
}

type containerTop struct {
	Name    string
	LimitMb int
	Procs   byMemory
}

type proc struct {
	Pid     string
	Memory  string
	Command string
}

type byMemory []proc

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func (ct containerTop) New(name string, limit int) containerTop {
	ct.Name = name
	ct.LimitMb = limit / 1024 / 1024
	ct.Procs = top(name)
	return ct
}

func getContainerTopByIp(ip string) containerTop {
	result := containerTop{}
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
			result = result.New(container.Name, limit)
			break
		}
	}
	if result.Name == "" {
		logger.Error("Cannot associate resolved IP to container")
	}
	return result
}

func handleTopPage(w http.ResponseWriter, r *http.Request) {
	containerIPs, err := net.LookupIP(r.Host)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	ct := getContainerTopByIp(fmt.Sprint(containerIPs[0]))
	err = tem.Execute(w, ct)
	if err != nil {
		logger.Panic(err.Error())
	}
}

func handleKill(w http.ResponseWriter, r *http.Request) {
	url := strings.Split(strings.Trim(string(r.URL.Path), "/"), "/")
	pid, err := strconv.Atoi(url[1])
	if err != nil {
		logger.Panic(err.Error())
	}
	process, err := gopsutil.NewProcess(int32(pid))
	if err != nil {
		logger.Error(err.Error())
	}
	process.Kill()

}

func Webserver(config *Config) {
	http.HandleFunc("/", handleTopPage)
	http.HandleFunc("/kill/", handleKill)
	http.ListenAndServe(fmt.Sprintf(":%d", config.HostPort), nil)
}

func top(container string) byMemory {
	res := byMemory{}

	pids, err := lxc.GetPids(container)
	if err != nil {
		logger.Panic(err.Error())
	}

	for _, pid := range pids {
		pr := proc{}
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

func (a byMemory) Len() int {
	return len(a)
}

func (a byMemory) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a byMemory) Less(i, j int) bool {
	ai, err := strconv.Atoi(a[i].Memory)
	if err != nil {
		logger.Panic(err.Error())
	}
	aj, err := strconv.Atoi(a[j].Memory)
	if err != nil {
		logger.Panic(err.Error())
	}
	return ai < aj
}
