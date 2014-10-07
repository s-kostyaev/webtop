package main

import (
	"fmt"
	"github.com/brnv/go-heaver"
	"github.com/s-kostyaev/go-lxc"
	"github.com/shirou/gopsutil"
	"html/template"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	LookupTimeout duration
	HostPort      int
}

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

type myTemplate struct {
	Template *template.Template
}

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

func (template myTemplate) handleTop(w http.ResponseWriter, r *http.Request) {
	ct := containerTop{}
	containerIPs, err := net.LookupIP(r.Host)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	containerIP := fmt.Sprint(containerIPs[0])
	containers, err := heaver.List("local")
	if err != nil {
		logger.Error(err.Error())
	}
	for _, container := range containers {
		if container.Ip == containerIP {
			limit, err := lxc.GetMemoryLimit(container.Name)
			if err != nil {
				logger.Error(err.Error())
			}
			ct = ct.New(container.Name, limit)
			break
		}
	}
	if ct.Name == "" {
		logger.Error("Cannot associate resolved IP to container")
		return
	}
	url := strings.Split(strings.Trim(string(r.URL.Path), "/"), "/")
	if url[0] == "kill" {
		pid, err := strconv.Atoi(url[1])
		if err != nil {
			logger.Panic(err.Error())
		}
		process, err := gopsutil.NewProcess(int32(pid))
		if err != nil {
			logger.Panic(err.Error())
		}
		process.Kill()
	}
	err = template.Template.Execute(w, ct)
	if err != nil {
		logger.Panic(err.Error())
	}
}

func Webserver(config *Config) {
	var tem myTemplate
	var err error
	tem.Template, err = template.ParseFiles(templatePath)
	if err != nil {
		logger.Panic(err)
	}
	http.HandleFunc("/", tem.handleTop)
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
