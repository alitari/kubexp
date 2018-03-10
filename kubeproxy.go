package kubeExplorer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type containerPort struct {
	name string
	port int
}

type portMapping struct {
	destPort      int
	containerPort containerPort
}

type portforwardProxy struct {
	mapping    portMapping
	pod        string
	namespace  string
	startTime  time.Time
	logChannel chan string
	log        bytes.Buffer
}

func newPortforwardProxy(namespace, podName string, ports portMapping) *portforwardProxy {
	p := &portforwardProxy{namespace: namespace, pod: podName, mapping: ports, logChannel: make(chan string), startTime: time.Now()}

	return p
}

func (p *portforwardProxy) execute() error {
	go func() {
		cmd := kubectl("port-forward", "-n", p.namespace, p.pod, fmt.Sprintf("%v:%v", p.mapping.destPort, p.mapping.containerPort.port))
		cmd.Output()
	}()
	return nil
}

func (p *portforwardProxy) getPid() int {
	return pidOfPort(p.mapping.destPort)
}

func (p *portforwardProxy) stop() error {
	pid := pidOfPort(p.mapping.destPort)
	if pid != -1 {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		if err := process.Kill(); err != nil {
			return err
		}
	}
	return nil
}

func kubectl(a1 string, a ...string) *exec.Cmd {
	full := append([]string{a1}, a[:]...)
	return execCommand("kubectl", full...)
}
