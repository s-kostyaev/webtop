package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/s-kostyaev/iptables/proxy"
	"github.com/s-kostyaev/lxc"
	"github.com/s-kostyaev/lxc/memory/monitor"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	config := GetConfig()
	go webserver()
	lookupTimeout, err := time.ParseDuration(config.LookupTimeout)
	if err != nil {
		log.Fatal(err)
	}
	monitorTimeout, err := time.ParseDuration(config.MonitorTimeout)
	if err != nil {
		log.Fatal(err)
	}
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
				go EnableRedirect(*memMon, monitorTimeout,
					enabledProxies)
			}

		}
		time.Sleep(lookupTimeout)
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
	pause time.Duration, enabledProxies map[string]bool) {
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
		time.Sleep(pause)
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
			log.Printf("Memory of %s availible. ",
				"Redirect to webtop disabled\n",
				memoryMon.cgroup.Container.Name)
			return
		}
	}
}

func getHostIP() (hostIP string) {
	cmd := exec.Command("hostname")
	cmd.Stdin = strings.NewReader("")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	host := strings.Trim(out.String(), "\n")
	hostIPs, err := net.LookupIP(host)
	if err != nil {
		log.Fatal(err)
	}
	return fmt.Sprint(hostIPs[0])
}

type Configuration struct {
	LookupTimeout  string
	MonitorTimeout string
	HostPort       int
}

func GetConfig() *Configuration {
	file, err := os.Open("/etc/webtop.json")
	if err != nil {
		log.Fatal(err)
	}
	decoder := json.NewDecoder(file)
	config := Configuration{}
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal(err)
	}
	return &config
}
func handler(w http.ResponseWriter, r *http.Request) {
	var mon *monitor.Cgroup
	containerIPs, err := net.LookupIP(r.Host)
	if err != nil {
		log.Println(err)
		return
	}
	containerIP := fmt.Sprint(containerIPs[0])
	var containerName string
	containers := lxc.GetContainers()
	for _, container := range containers {
		if container.IP == containerIP {
			containerName = container.Name
			mon, err = monitor.NewCgroup(container,
				monitor.PREFIX,
				monitor.LIMIT,
				monitor.USAGE,
			)
			if err != nil {
				log.Println(err)
				return
			}
		}
	}
	if containerName == "" {
		log.Println("Cannot associate resolved IP to container")
		return
	}
	limit, err := mon.Get("limit")
	if err != nil {
		log.Println(err)
	}
	url := strings.Split(strings.Trim(string(r.URL.Path), "/"), "/")
	if url[0] == "kill" {
		kill(url[1])
	}
	procs := Top(containerName)
	fmt.Fprintln(w, "<style>tbody tr:hover {\nbackground: #FFEBCD; ")
	fmt.Fprintln(w, "/* Цвет фона при наведении */\n}</style>")
	fmt.Fprintln(w, "<body>")
	fmt.Fprint(w, "<h1 style=\"font-size: 2em; font-family: Ubuntu\"")
	fmt.Fprintf(w, " align=center>Память %s исчерпана</h1>", containerName)
	fmt.Fprint(w, "<h3 style=\"font-size: 1.2em; font-family: Ubuntu\"")
	fmt.Fprint(w, " align=center>Память")
	fmt.Fprintf(w, "%s достигла порогового значения %d Mb. ",
		containerName, limit/1024/1024)
	fmt.Fprint(w, "Для продолжения работы необходимо завершить один из ",
		"процессов</h3>")
	fmt.Fprintln(w, "<table width=80% align=center cellspacing=0 ",
		"cellpadding=3 style=\"font-size: 1.2em; ",
		"font-family: Ubuntu\">")
	fmt.Fprintln(w, "<thead><tr><td><b>PID</b></td><td><b>Используемая",
		" память</b></td><td><b>Команда</b></td></tr></thead>")
	fmt.Fprintln(w, "<tbody>")
	for _, proc := range procs {
		fmt.Fprintln(w, "<tr>")
		for i, field := range proc {
			if i == 1 {
				mem, err := strconv.Atoi(field)
				if err != nil {
					log.Println(err)
				}
				fmt.Fprintf(w, "<td>%d Mb</td>", mem/1024)
				continue
			}
			fmt.Fprintf(w, "<td>%s</td>", field)
		}
		if !strings.Contains(proc[2], "/sbin/init") {
			fmt.Fprint(w, "<td><a href=/kill/")
			fmt.Fprintf(w, "%s>завершить</a></td>", proc[0])
		}
		fmt.Fprintln(w, "</tr>")

	}
	fmt.Fprintln(w, "</tbody>")
	fmt.Fprintln(w, "</table>")
	fmt.Fprintln(w, "</body>")
}

func webserver() {
	config := GetConfig()
	http.HandleFunc("/", handler)
	http.ListenAndServe(fmt.Sprintf(":%d", config.HostPort), nil)
}

func Top(container string) [][]string {
	cmd1 := exec.Command("ps", "-o", "pid,rss,args,cgroup",
		"-k", "-rss", "-ax")
	cmd2 := exec.Command("grep", container)

	r1, w1 := io.Pipe()

	cmd1.Stdout = w1
	cmd2.Stdin = r1

	var out bytes.Buffer
	cmd2.Stdout = &out

	cmd1.Start()
	cmd2.Start()
	cmd1.Wait()
	w1.Close()
	cmd2.Wait()

	var res [][]string

	results := strings.Split(strings.Trim(out.String(), "\n"), "\n")
	for _, result := range results {
		var tmp []string
		buf := strings.Fields(result)
		tmp = buf[:2]
		tmp = append(tmp, strings.Join(buf[2:len(buf)-1], " "))
		res = append(res, tmp)
	}
	return res
}

func kill(pid string) {
	cmd := exec.Command("kill", "-9", pid)
	cmd.Start()
	cmd.Wait()
}
