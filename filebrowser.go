package kubexp

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type fileBrowserType interface {
	getFileList(filename string) []interface{}
	getContext() string
	getPath(filename string) string
	isSourceSelection() bool
}

type localFileBrowser struct {
	currentDir      string
	podContext      string
	sourceSelection bool
}

func (f *localFileBrowser) getPath(filename string) string {
	return filepath.Join(f.currentDir, filename)
}

func (f *localFileBrowser) isSourceSelection() bool {
	return f.sourceSelection
}

func (f *localFileBrowser) getContext() string {
	fp, _ := filepath.Abs(f.currentDir)
	if len(fp) > 30 {
		fp = "..." + fp[len(fp)-30:]
	}
	if f.isSourceSelection() {
		return fmt.Sprintf("Select source file, dir: %-35.35s", fp)
	} else {
		return fmt.Sprintf("Select destination dir: %-35.35s", fp)
	}
}

func (f *localFileBrowser) getFileList(filename string) []interface{} {
	f.currentDir = filepath.Join(f.currentDir, filename)
	fileList := []interface{}{}
	if !f.isSourceSelection() {
		fileList = append(fileList, map[string]interface{}{"dir": false, "name": "Select this dir", "mode": "", "size": 0, "time": ""})
	}
	fileList = append(fileList, map[string]interface{}{"dir": true, "name": "..", "mode": "", "size": 0, "time": ""})

	files, err := ioutil.ReadDir(f.currentDir)
	if err != nil {
		fatalStderrlog.Fatal(err)
	}

	for _, f := range files {
		timef := f.ModTime().Format(time.RFC3339)
		fileList = append(fileList, map[string]interface{}{"dir": f.IsDir(), "name": f.Name(), "mode": f.Mode().String(), "size": f.Size(), "time": timef})
	}
	return fileList
}

type podFileBrowser struct {
	currentDir      string
	namespace       string
	podName         string
	sourceSelection bool
}

func (p *podFileBrowser) getPath(filename string) string {
	return path.Join(p.currentDir, filename)
}

func (p *podFileBrowser) isSourceSelection() bool {
	return p.sourceSelection
}

func (p *podFileBrowser) getContext() string {
	fp := p.currentDir
	if len(fp) > 30 {
		fp = "..." + fp[len(fp)-30:]
	}
	if p.isSourceSelection() {
		return fmt.Sprintf("Select source file, dir: %-35.35s", fp)
	} else {
		return fmt.Sprintf("Select destination dir: %-35.35s", fp)
	}
}

func (p *podFileBrowser) getFileList(filename string) []interface{} {

	p.currentDir = path.Join(p.currentDir, filename)
	fileList := []interface{}{}
	if !p.isSourceSelection() {
		fileList = append(fileList, map[string]interface{}{"dir": false, "name": "Select this dir", "mode": "", "size": 0, "time": ""})
	}

	cmd := kubectl("-n", p.namespace, "exec", p.podName, "--", "ls", "-l", "-a", p.currentDir)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		fullmess := fmt.Sprintf("%v: %s", err, stderr.String())
		errorlog.Print(fullmess)
		showError(fullmess, err)
	}
	lsStr := out.String()
	lines := strings.Split(lsStr, "\n")

	for _, l := range lines {
		fileFields := strings.Fields(l)
		if len(fileFields) > 4 {
			size, _ := strconv.Atoi(fileFields[4])
			isDir := fileFields[0][0] == 'd'
			fileList = append(fileList, map[string]interface{}{"dir": isDir, "name": fileFields[len(fileFields)-1], "mode": fileFields[0], "size": size, "time": ""})
		}
	}
	return fileList
}
