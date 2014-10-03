package main

import (
	"bytes"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/s-kostyaev/lxc"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"os/exec"
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

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func GetConfig(configPath string) *Config {
	buf, err := ioutil.ReadFile(configPath)
	if err != nil {
		Log.Fatal(err.Error())
	}
	config := Config{}
	_, err = toml.Decode(string(buf), &config)
	if err != nil {
		Log.Fatal(err.Error())
	}
	return &config
}

type containerTop struct {
	Name    string
	LimitMb int
	Procs   []proc
}

type proc struct {
	Pid     string
	Memory  string
	Command string
}

func newProc(src []string) (proc proc) {
	proc.Pid = src[0]
	mem, err := strconv.Atoi(src[1])
	if err != nil {
		Log.Error(err.Error())
	}
	proc.Memory = fmt.Sprint(mem / 1024)
	proc.Command = src[2]
	return proc
}

func (ct containerTop) New(name string, limit int) containerTop {
	ct.Name = name
	ct.LimitMb = limit / 1024 / 1024
	ct.Procs = top(name)
	return ct
}

type myTemplate struct {
	Template *template.Template
}

func (template myTemplate) handler(w http.ResponseWriter, r *http.Request) {
	ct := containerTop{}
	containerIPs, err := net.LookupIP(r.Host)
	if err != nil {
		Log.Error(err.Error())
		return
	}
	containerIP := fmt.Sprint(containerIPs[0])
	containers, err := lxc.GetContainers()
	if err != nil {
		Log.Error(err.Error())
	}
	for _, container := range containers {
		if container.IP == containerIP {
			limit, err := lxc.GetParamInt("memory",
				container.Name, "limit")
			if err != nil {
				Log.Error(err.Error())
			}
			ct = ct.New(container.Name, limit)
			break
		}
	}
	if ct.Name == "" {
		Log.Error("Cannot associate resolved IP to container")
		return
	}
	url := strings.Split(strings.Trim(string(r.URL.Path), "/"), "/")
	if url[0] == "kill" {
		kill(url[1])
	}
	err = template.Template.Execute(w, ct)
	if err != nil {
		Log.Panic(err.Error())
	}
}

func Webserver(config *Config) {
	var tem myTemplate
	var err error
	tem.Template, err = template.ParseFiles(TemplatePath)
	if err != nil {
		Log.Panic(err)
	}
	http.HandleFunc("/", tem.handler)
	http.ListenAndServe(fmt.Sprintf(":%d", config.HostPort), nil)
}

func top(container string) []proc {
	cmd := exec.Command("ps", "-o", "pid,rss,args,cgroup",
		"-k", "-rss", "-ax")

	cmd.Stdout = &bytes.Buffer{}
	err := cmd.Run()
	if err != nil {
		Log.Error(err.Error())
	}

	res := []proc{}

	results := strings.Split(
		strings.Trim(cmd.Stdout.(*bytes.Buffer).String(), "\n"), "\n")
	for _, result := range results {
		tmp := []string{}
		buf := strings.Fields(result)
		if strings.Contains(buf[len(buf)-1], container) {
			tmp = buf[:2]
			tmp = append(tmp, strings.Join(buf[2:len(buf)-1], " "))
			res = append(res, newProc(tmp))
		}
	}
	return res
}

func kill(pid string) {
	cmd := exec.Command("kill", "-9", pid)
	err := cmd.Run()
	if err != nil {
		Log.Panic(err.Error())
	}
}
