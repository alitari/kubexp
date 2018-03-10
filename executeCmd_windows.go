// +build windows

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
	fullargs := append([]string{"/C"}, name)
	fullargs = append(fullargs, args[:]...)
	return exec.Command("cmd", fullargs...)
}

func pidOfPort(port int) int {
	cmd := execCommand("netstat", "-aon")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	outStr := out.String()
	ts := strings.Split(outStr, "\r\n")
	for _, s := range ts {
		if strings.Contains(s, fmt.Sprintf("127.0.0.1:%v", port)) {
			li := strings.LastIndex(s, " ")
			// log.Printf("found: %s \npid: %s", s, s[li+1:])
			pid, err := strconv.Atoi(s[li+1:])
			if err != nil {
				log.Fatal(err)
			}
			return pid
		}
	}
	return -1
}
