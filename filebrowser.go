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

type fileBrowserType struct {
	local           bool
	sourceSelection bool
	currentDir      string
	createFileParts func(filename string) []interface{}
}

func newLocalFileBrowser(sourceSelection bool, dir string) *fileBrowserType {
	return &fileBrowserType{local: true, sourceSelection: sourceSelection, currentDir: dir, createFileParts: createLocalFileParts}
}

func newRemoteFileBrowser(sourceSelection bool, dir string) *fileBrowserType {
	return &fileBrowserType{local: false, sourceSelection: sourceSelection, currentDir: dir, createFileParts: createRemoteFileParts}
}

func (f *fileBrowserType) getPath(filename string) string {
	return path.Join(f.currentDir, filename)
}

func (f *fileBrowserType) getContext() string {
	fp := f.currentDir
	if len(fp) > 30 {
		fp = "..." + fp[len(fp)-30:]
	}
	if f.sourceSelection {
		return fmt.Sprintf("Select source file, dir: %-35.35s", fp)
	}
	return fmt.Sprintf("Select destination dir: %-35.35s", fp)
}

func (f *fileBrowserType) getFileList(filename string) []interface{} {
	f.currentDir = path.Join(f.currentDir, filename)
	fileList := []interface{}{}
	if !f.sourceSelection {
		fileList = append(fileList, map[string]interface{}{"dir": false, "name": "Select this dir", "mode": "", "size": 0, "time": ""})
		fileList = append(fileList, filterDirs(f.createFileParts(f.currentDir))...)
	} else {
		fileList = append(fileList, f.createFileParts(f.currentDir)...)
	}

	return fileList
}

func filterDirs(files []interface{}) (ret []interface{}) {
	for _, f := range files {
		fm := f.(map[string]interface{})
		if fm["dir"].(bool) {
			ret = append(ret, f)
		}
	}
	return ret
}

func createLocalFileParts(file string) []interface{} {
	fileList := []interface{}{map[string]interface{}{"dir": true, "name": "..", "mode": "", "size": 0, "time": ""}}
	absPath, _ := filepath.Abs(file)
	files, err := ioutil.ReadDir(absPath)
	if err != nil {
		fatalStderrlog.Fatal(err)
	}

	for _, f := range files {
		timef := f.ModTime().Format(time.RFC3339)
		fileList = append(fileList, map[string]interface{}{"dir": f.IsDir(), "name": f.Name(), "mode": f.Mode().String(), "size": f.Size(), "time": timef})
	}
	return fileList
}

func createRemoteFileParts(file string) []interface{} {
	fileList := []interface{}{}
	podName := selectedResourceItemName()
	ns := selectedResourceItemNamespace()
	cmd := kubectl("-n", ns, "exec", podName, "--", "ls", "-l", "-a", file)
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

// func (p *podFileBrowser) getFileList(filename string) []interface{} {
// 	p.currentDir = path.Join(p.currentDir, filename)
// 	fileList := []interface{}{}
// 	if !p.isSourceSelection() {
// 		fileList = append(fileList, map[string]interface{}{"dir": false, "name": "Select this dir", "mode": "", "size": 0, "time": ""})
// 	}

// 	cmd := kubectl("-n", p.namespace, "exec", p.podName, "--", "ls", "-l", "-a", p.currentDir)
// 	var out bytes.Buffer
// 	var stderr bytes.Buffer
// 	cmd.Stdout = &out
// 	cmd.Stderr = &stderr
// 	err := cmd.Run()
// 	if err != nil {
// 		fullmess := fmt.Sprintf("%v: %s", err, stderr.String())
// 		errorlog.Print(fullmess)
// 		showError(fullmess, err)
// 	}
// 	lsStr := out.String()
// 	lines := strings.Split(lsStr, "\n")

// 	for _, l := range lines {
// 		fileFields := strings.Fields(l)
// 		if len(fileFields) > 4 {
// 			size, _ := strconv.Atoi(fileFields[4])
// 			isDir := fileFields[0][0] == 'd'
// 			fileList = append(fileList, map[string]interface{}{"dir": isDir, "name": fileFields[len(fileFields)-1], "mode": fileFields[0], "size": size, "time": ""})
// 		}
// 	}
// 	return fileList
// }
