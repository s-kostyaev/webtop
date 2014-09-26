package main

import (
	"bytes"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/s-kostyaev/iptables/proxy"
	"github.com/s-kostyaev/lxc"
	"github.com/s-kostyaev/lxc/memory/monitor"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	go webserver()
	lookup()
}

func lookup() {
	config := GetConfig()
	hostIP := getHostIP()
	enabledProxies := make(map[string]bool)
	for {
		containers := lxc.GetContainers()
		for _, container := range containers {
			mon, err := monitor.NewCgroup(
				container,
				monitor.PREFIX,
				monitor.LIMIT,
				monitor.USAGE,
			)
			if err != nil {
				log.Println(err)
				continue
			}
			limit, err := mon.Get("limit")
			if err != nil {
				log.Println(err)
				continue
			}
			usage, err := mon.Get("usage")
			if err != nil {
				log.Println(err)
				continue
			}
			if limit == usage {
				memMon := NewMemoryMonitor(mon, hostIP,
					config.HostPort)
				go EnableRedirect(*memMon,
					config.MonitorTimeout,
					enabledProxies)
			}

		}
		time.Sleep(config.LookupTimeout.Duration)
	}
}

type MemoryMonitor struct {
	cgroup *monitor.Cgroup
	proxy  *proxy.Proxy
}

func NewMemoryMonitor(cgroup *monitor.Cgroup,
	hostIP string, hostPort int) *MemoryMonitor {
	var memMon MemoryMonitor
	memMon.cgroup = cgroup
	memMon.proxy = proxy.NewProxy(cgroup.Container.IP, 80, hostIP, hostPort)
	return &memMon
}

func EnableRedirect(memoryMon MemoryMonitor,
	pause duration, enabledProxies map[string]bool) {
	if enabledProxies[memoryMon.cgroup.Container.Name] == true {
		return
	}
	log.Printf("Memory of %s has reached the limit. ",
		memoryMon.cgroup.Container.Name)
	enabledProxies[memoryMon.cgroup.Container.Name] = true
	err := memoryMon.proxy.Enable()
	if err != nil {
		log.Println(err)
	}
	defer memoryMon.proxy.Disable()
	for {
		time.Sleep(time.Duration(pause.Duration))
		limit, err := memoryMon.cgroup.Get("limit")
		if err != nil {
			log.Println(err)
			continue
		}
		usage, err := memoryMon.cgroup.Get("usage")
		if err != nil {
			log.Println(err)
			continue
		}
		if limit != usage {
			enabledProxies[memoryMon.cgroup.Container.Name] = false
			log.Printf("Memory of %s availible. "+
				"Redirect to webtop disabled\n",
				memoryMon.cgroup.Container.Name)
			return
		}
	}
}

func getHostIP() (hostIP string) {
	hosts, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Split(hosts[1].String(), "/")[0]
}

type Config struct {
	LookupTimeout  duration
	MonitorTimeout duration
	HostPort       int
}

type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func GetConfig() *Config {
	buf, err := ioutil.ReadFile("/etc/webtop.toml")
	if err != nil {
		log.Fatal(err)
	}
	config := Config{}
	_, err = toml.Decode(string(buf), &config)
	if err != nil {
		log.Fatal(err)
	}
	return &config
}

type ContainerTop struct {
	Name    string
	LimitMb int
	Procs   []Proc
}

type Proc struct {
	Pid     string
	Memory  string
	Command string
}

func NewProc(src []string) (proc Proc) {
	proc.Pid = src[0]
	mem, err := strconv.Atoi(src[1])
	if err != nil {
		log.Panic(err)
	}
	proc.Memory = fmt.Sprint(mem / 1024)
	proc.Command = src[2]
	return proc
}

func (ct ContainerTop) New(name string, limit int) ContainerTop {
	ct.Name = name
	ct.LimitMb = limit / 1024 / 1024
	ct.Procs = Top(name)
	return ct
}

func handler(w http.ResponseWriter, r *http.Request) {
	var mon *monitor.Cgroup
	var ct ContainerTop
	containerIPs, err := net.LookupIP(r.Host)
	if err != nil {
		log.Println(err)
		return
	}
	containerIP := fmt.Sprint(containerIPs[0])
	containers := lxc.GetContainers()
	for _, container := range containers {
		if container.IP == containerIP {
			mon, err = monitor.NewCgroup(container,
				monitor.PREFIX,
				monitor.LIMIT,
				monitor.USAGE,
			)
			if err != nil {
				log.Println(err)
				return
			}
			limit, err := mon.Get("limit")
			if err != nil {
				log.Println(err)
			}
			ct = ct.New(container.Name, limit)
		}
	}
	if ct.Name == "" {
		log.Println("Cannot associate resolved IP to container")
		return
	}
	url := strings.Split(strings.Trim(string(r.URL.Path), "/"), "/")
	if url[0] == "kill" {
		kill(url[1])
	}
	tem, err := template.ParseFiles("top.htm")
	if err != nil {
		log.Panic(err)
	}
	err = tem.Execute(w, ct)
	if err != nil {
		log.Panic(err)
	}
}

func webserver() {
	config := GetConfig()
	http.HandleFunc("/", handler)
	http.ListenAndServe(fmt.Sprintf(":%d", config.HostPort), nil)
}

func Top(container string) []Proc {
	cmd := exec.Command("ps", "-o", "pid,rss,args,cgroup",
		"-k", "-rss", "-ax")

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Println(err)
	}

	var res []Proc

	results := strings.Split(strings.Trim(out.String(), "\n"), "\n")
	for _, result := range results {
		var tmp []string
		buf := strings.Fields(result)
		if strings.Contains(buf[len(buf)-1], container) {
			tmp = buf[:2]
			tmp = append(tmp, strings.Join(buf[2:len(buf)-1], " "))
			res = append(res, NewProc(tmp))
		}
	}
	return res
}

func kill(pid string) {
	cmd := exec.Command("kill", "-9", pid)
	err := cmd.Run()
	if err != nil {
		log.Panicln(err)
	}
}
