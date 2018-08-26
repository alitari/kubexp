package kubexp

import (
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
	if len(fp) > 20 {
		fp = "..." + fp[len(fp)-20:]
	}
	podName := selectedResourceItemName()
	containerName := containerNames[selectedContainerIndex]

	if f.sourceSelection {
		if f.local {
			return fmt.Sprintf("Select source file, dir: %-20.20s", fp)
		}
		return fmt.Sprintf("pod/container: %17.17s/%-15.15s dir: %-20.20s", podName, containerName, fp)
	}
	if f.local {
		return fmt.Sprintf("Select destination dir: %-35.35s", fp)
	}
	return fmt.Sprintf("pod/container: %17.17s/%-15.15s dir: %-20.20s", podName, containerName, fp)
}

func (f *fileBrowserType) getFileList(filename string) []interface{} {
	f.currentDir = path.Join(f.currentDir, filename)
	fileList := []interface{}{}
	if !f.sourceSelection {
		fileList = append(fileList, map[string]interface{}{"dir": false, "name": "Select this dir", "mode": "", "size": "0", "time": ""})
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
	fileList := []interface{}{map[string]interface{}{"dir": true, "name": "..", "mode": "", "size": "0", "time": ""}}
	absPath, _ := filepath.Abs(file)
	files, err := ioutil.ReadDir(absPath)
	if err != nil {
		fatalStderrlog.Fatal(err)
	}

	for _, f := range files {
		timef := f.ModTime().Format(time.RFC3339)
		fileList = append(fileList, map[string]interface{}{"dir": f.IsDir(), "name": f.Name(), "mode": f.Mode().String(), "size": strconv.FormatInt(f.Size(), 10), "time": timef})
	}
	return fileList
}

func createRemoteFileParts(file string) []interface{} {
	fileList := []interface{}{}
	podName := selectedResourceItemName()
	ns := selectedResourceItemNamespace()
	con := containerNames[selectedContainerIndex]
	cmd := kubectl(backend.context.Name, "-n", ns, "exec", podName, "-c", con, "--", "ls", "-l", "-a", file)
	lsStr, errorStr, err := runCmd(cmd)
	if err != nil {
		fullmess := fmt.Sprintf("%v: %s", err, errorStr)
		errorlog.Print(fullmess)
		showError(fullmess, err)
	}
	lines := strings.Split(lsStr, "\n")

	for _, l := range lines {
		fileFields := strings.Fields(l)
		if len(fileFields) > 7 {
			isDir := fileFields[0][0] == 'd'
			time := fmt.Sprintf("%3.3s %2.2s %5.5s ", fileFields[5], fileFields[6], fileFields[7])
			fileList = append(fileList, map[string]interface{}{"dir": isDir, "name": fileFields[len(fileFields)-1], "mode": fileFields[0], "size": fileFields[4], "time": time})
		}
	}
	return fileList
}
