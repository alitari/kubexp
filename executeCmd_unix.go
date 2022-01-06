//go:build linux || darwin
// +build linux darwin

package kubexp

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func pidOfPort(port int) int {
	cmd := execCommand("netstat", "-tulpn")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	outStr := out.String()
	ts := strings.Split(outStr, "\n")
	for _, s := range ts {
		if strings.Contains(s, fmt.Sprintf("127.0.0.1:%v", port)) {
			li := strings.LastIndex(s, "LISTEN")
			sub := strings.Trim(s[li+6:], " ")
			li2 := strings.LastIndex(sub, "/")
			//log.Printf("found: %s \n sub: %s pid: %s", s, sub, sub[:li2])
			pid, err := strconv.Atoi(sub[:li2])
			if err != nil {
				log.Fatal(err)
			}
			return pid
		}
	}
	return -1
}
