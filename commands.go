package kubexp

import (
	"fmt"

	"github.com/alitari/gocui"
)

type commandType struct {
	Name string
	f    func(g *gocui.Gui, v *gocui.View) error
}

type keyEventType struct {
	Viewname string
	Key      interface{}
	mod      gocui.Modifier
}

type keyBindingType struct {
	show     bool
	KeyEvent keyEventType
	Command  commandType
}

func newScaleCommand(name string, replicas int) commandType {
	var scaleCommand = commandType{Name: name, f: func(g *gocui.Gui, v *gocui.View) error {
		res := selectedResource()
		if res.Name == "deployments" || res.Name == "replicationcontrollers" || res.Name == "replicasets" || res.Name == "daemonsets" || res.Name == "statefulsets" {
			scaleResource(replicas)
		}
		return nil
	}}
	return scaleCommand
}

func newExecCommand(name, cmd string, containerNr int) commandType {
	var execCommand = commandType{Name: name, f: func(g *gocui.Gui, v *gocui.View) error {
		res := selectedResource()
		if res.Name != "pods" {
			return nil
		}
		ns := selectedResourceItemNamespace()
		rname := selectedResourceItemName()
		details := resourceItemsList.widget.items[resourceItemsList.widget.selectedItem]
		containerNames = resItemContainers(details)
		if containerNr > len(containerNames)-1 {
			containerNr = 0
		}

		cmd := kubectl(backend.context.Name, "-n", ns, "exec", "-c", containerNames[containerNr], "-it", rname, cmd)

		exe <- cmd
		return gocui.ErrQuit
	}}
	return execCommand
}

func newPortForwardCommand(name string, useSamePort bool) commandType {

	var portForwardCommand = commandType{Name: name, f: func(g *gocui.Gui, v *gocui.View) error {
		res := selectedResource()
		if res.Name != "pods" {
			return nil
		}
		podName := selectedResourceItemName()
		ns := selectedResourceItemNamespace()
		if portforwardProxies[ns+"/"+podName] != nil {
			err := removePortforwardProxyofPod(ns, podName)
			if err != nil {
				showError("Can't remove port-forward proxy", err)
				return nil
			}
			return nil
		}
		pod := resourceItemsList.widget.items[resourceItemsList.widget.selectedItem]

		ports := ports(pod)
		for _, cp := range ports {
			var localPort int
			if useSamePort {
				localPort = cp.port
			} else {
				localPort = currentPortforwardPort
				currentPortforwardPort++
			}
			err := createPortforwardProxy(ns, podName, portMapping{localPort, cp})
			if err != nil {
				showError("Can't create port-forward proxy", err)
				return nil
			}
		}
		return nil
	}}
	return portForwardCommand
}

var portForwardSamePortCommand = newPortForwardCommand("toggle port forward (same port)", true)
var portForwardCommand = newPortForwardCommand("toggle port forward", false)

var quitCommand = commandType{Name: "Quit", f: func(g *gocui.Gui, v *gocui.View) error {
	removeAllPortforwardProxies()
	leaveApp = true
	return gocui.ErrQuit
}}

var nextResourceCommand = commandType{Name: "Next resource", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceMenu.widget.nextSelectedItem()
	newResource()
	return nil
}}
var previousResourceCommand = commandType{Name: "Previous resource", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceMenu.widget.previousSelectedItem()
	newResource()
	return nil
}}

func newConfirmCommand(name string, command commandType) commandType {
	var confirmCommand = commandType{Name: name, f: func(g *gocui.Gui, v *gocui.View) error {
		showConfirm(fmt.Sprintf("Delete %s from %s ?", selectedResourceItemName(), selectedResource().Name), command)
		return nil
	}}
	return confirmCommand
}

func newLoadingCommand(name string, command commandType) commandType {
	var loadingCommand = commandType{Name: name, f: func(g *gocui.Gui, v *gocui.View) error {
		showLoading(command, g, v)
		return nil
	}}
	return loadingCommand
}

var executeConfirmCommand = commandType{Name: "Execute command to confirm", f: func(g *gocui.Gui, v *gocui.View) error {
	quitWidgetCommand.f(g, v)
	confirmCommand.f(g, v)
	return nil
}}

var deleteCommand = commandType{Name: "Delete resource", f: func(g *gocui.Gui, v *gocui.View) error {
	deleteResource(false)
	return nil
}}

var deleteNoGracePeriodCommand = commandType{Name: "Delete resource", f: func(g *gocui.Gui, v *gocui.View) error {
	deleteResource(true)
	return nil
}}

var deleteConfirmCommand = newConfirmCommand("Delete resource", deleteCommand)
var deleteNoGracePeriodConfirmCommand = newConfirmCommand("Delete resource immediately", deleteNoGracePeriodCommand)

var nameSortCommand = commandType{Name: "Sort by name", f: func(g *gocui.Gui, v *gocui.View) error {
	nameSorting()
	newResource()
	return nil
}}

var ageSortCommand = commandType{Name: "Sort by age", f: func(g *gocui.Gui, v *gocui.View) error {
	ageSorting()
	newResource()
	return nil
}}

var scaleUpCommand = newScaleCommand("Scale up", 1)
var scaleDownCommand = newScaleCommand("Scale down", -1)

var execBashCommand0 = newExecCommand("Exec first container bash", "bash", 0)
var execBashCommand1 = newExecCommand("Exec second container bash", "bash", 1)
var execBashCommand2 = newExecCommand("Exec third container bash", "bash", 2)
var execShellCommand0 = newExecCommand("Exec first container sh", "sh", 0)
var execShellCommand1 = newExecCommand("Exec second container sh", "sh", 1)
var execShellCommand2 = newExecCommand("Exec third container sh", "sh", 2)

var nextResourceItemDetailPartCommand = commandType{Name: "Next detail part", f: func(g *gocui.Gui, v *gocui.View) error {
	leaveResourceItemDetailsPart()
	resourcesItemDetailsMenu.widget.nextSelectedItem()
	setResourceItemDetailsPart()
	return nil
}}

var previousResourceItemDetailPartCommand = commandType{Name: "Previous detail part", f: func(g *gocui.Gui, v *gocui.View) error {
	leaveResourceItemDetailsPart()
	resourcesItemDetailsMenu.widget.previousSelectedItem()
	setResourceItemDetailsPart()
	return nil
}}

var reloadResourceItemDetailPartCommand = commandType{Name: "Reload detail part", f: func(g *gocui.Gui, v *gocui.View) error {
	reloadResourceItemDetailsPart()
	return nil
}}

var nextLineCommand = commandType{Name: "Next resource item", f: func(g *gocui.Gui, v *gocui.View) error {
	if resourceItemDetailsWidget.visible {
		return nil
	}
	resourceItemsList.widget.nextSelectedItem()
	return nil
}}
var previousLineCommand = commandType{Name: "Previous resource item", f: func(g *gocui.Gui, v *gocui.View) error {
	if resourceItemDetailsWidget.visible {
		return nil
	}
	resourceItemsList.widget.previousSelectedItem()
	return nil
}}

var nextResourceItemListPageCommand = commandType{Name: "Next resource item page ", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemsList.widget.nextPage()
	return nil
}}

var previousResourceItemListPageCommand = commandType{Name: "Previous resource item page ", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemsList.widget.previousPage()
	return nil
}}

var toggleResourceItemDetailsCommand = commandType{Name: "Toggle resource item details ", f: func(g *gocui.Gui, v *gocui.View) error {
	toggleDetailBrowseState()
	return nil
}}

var findNextCommand = commandType{Name: "find next on resource item details ", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.findNext()
	return nil
}}

var findPreviousCommand = commandType{Name: "find previous on resource item details ", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.findPrevious()
	return nil
}}

var homeCommand = commandType{Name: "TextArea home", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollUp(1<<63 - 1)
	return nil
}}

var endCommand = commandType{Name: "TextArea end", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollDown((1<<63 - 1) / 2)
	return nil
}}

var pageUpCommand = commandType{Name: "TextArea page up", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollUp(resourceItemDetailsWidget.h)
	return nil
}}

var pageDownCommand = commandType{Name: "TextArea page down", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollDown(resourceItemDetailsWidget.h)
	return nil
}}

var scrollUpCommand = commandType{Name: "TextArea scroll up", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollUp(1)
	return nil
}}

var scrollDownCommand = commandType{Name: "TextArea scroll down", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollDown(1)
	return nil
}}

var scrollUpHelpCommand = commandType{Name: "Help scroll up", f: func(g *gocui.Gui, v *gocui.View) error {
	helpWidget.scrollUp(1)
	return nil
}}

var scrollDownHelpCommand = commandType{Name: "Help scroll down", f: func(g *gocui.Gui, v *gocui.View) error {
	helpWidget.scrollDown(1)
	return nil
}}

var scrollRightCommand = commandType{Name: "TextArea scroll right", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollRight()
	return nil
}}
var scrollLeftCommand = commandType{Name: "TextArea scroll left", f: func(g *gocui.Gui, v *gocui.View) error {
	resourceItemDetailsWidget.scrollLeft()
	return nil
}}

var gotoSelectContextStateCommand = commandType{Name: "Select context", f: func(g *gocui.Gui, v *gocui.View) error {
	setState(selectContextState)
	return nil
}}

var loadContextCommand = newLoadingCommand("reload context", commandType{Name: "sc", f: func(g *gocui.Gui, v *gocui.View) error {
	newContext()
	return nil
}})

var nextContextCommand = commandType{Name: "Next context", f: func(g *gocui.Gui, v *gocui.View) error {
	clusterList.widget.nextSelectedItem()
	return nil
}}
var previousContextCommand = commandType{Name: "Previous context", f: func(g *gocui.Gui, v *gocui.View) error {
	clusterList.widget.previousSelectedItem()
	return nil
}}

var nextContextPageCommand = commandType{Name: "Next context", f: func(g *gocui.Gui, v *gocui.View) error {
	clusterList.widget.nextPage()
	return nil
}}

var previousContextPageCommand = commandType{Name: "Next context", f: func(g *gocui.Gui, v *gocui.View) error {
	clusterList.widget.previousPage()
	return nil
}}

var gotoSelectNamespaceStateCommand = commandType{Name: "Select namespace", f: func(g *gocui.Gui, v *gocui.View) error {
	if namespaceList.widget.visible {
		setState(selectNsState)
	}
	return nil
}}

var selectNamespaceLoadingCommand = newLoadingCommand("Select namespace", commandType{Name: "sn", f: func(g *gocui.Gui, v *gocui.View) error {
	findResourceCategoryWithResources(1)
	return nil
}})

var nextNamespaceCommand = commandType{Name: "Next namespace", f: func(g *gocui.Gui, v *gocui.View) error {
	namespaceList.widget.nextSelectedItem()
	return nil
}}

var previousNamespaceCommand = commandType{Name: "Previous namespace", f: func(g *gocui.Gui, v *gocui.View) error {
	namespaceList.widget.previousSelectedItem()
	return nil
}}

var nextNamespacePageCommand = commandType{Name: "Next namespace", f: func(g *gocui.Gui, v *gocui.View) error {
	namespaceList.widget.nextPage()
	return nil
}}

var previousNamespacePageCommand = commandType{Name: "Previous namespace", f: func(g *gocui.Gui, v *gocui.View) error {
	namespaceList.widget.previousPage()
	return nil
}}

var nextResourceCategoryCommand = commandType{Name: "Next resource category", f: func(g *gocui.Gui, v *gocui.View) error {
	nextResourceCategory(1)
	findResourceCategoryWithResources(1)
	return nil
}}

var previousResourceCategoryCommand = commandType{Name: "Next resource category", f: func(g *gocui.Gui, v *gocui.View) error {
	nextResourceCategory(-1)
	findResourceCategoryWithResources(-1)
	return nil
}}

var nextContainerCommand = commandType{Name: "Pod logs: Next container ", f: func(g *gocui.Gui, v *gocui.View) error {
	nextLogContainer()
	return nil
}}

var showHelpCommand = commandType{Name: "Show help", f: func(g *gocui.Gui, v *gocui.View) error {
	setState(helpState)
	return nil
}}

var quitWidgetCommand = commandType{Name: "quit", f: func(g *gocui.Gui, v *gocui.View) error {
	setState(browseState)
	return nil
}}

var uploadFileCommand = commandType{Name: "Upload file", f: func(g *gocui.Gui, v *gocui.View) error {
	startFiletransfer(true)
	return nil
}}

var downloadFileCommand = commandType{Name: "Download file", f: func(g *gocui.Gui, v *gocui.View) error {
	startFiletransfer(false)
	return nil
}}

var nextFileCommand = commandType{Name: "Next File", f: func(g *gocui.Gui, v *gocui.View) error {
	fileList.widget.nextSelectedItem()
	return nil
}}

var previousFileCommand = commandType{Name: "Previous File", f: func(g *gocui.Gui, v *gocui.View) error {
	fileList.widget.previousSelectedItem()
	return nil
}}

var nextFilePageCommand = commandType{Name: "Next File", f: func(g *gocui.Gui, v *gocui.View) error {
	fileList.widget.nextPage()
	return nil
}}

var previousFilePageCommand = commandType{Name: "Previous File", f: func(g *gocui.Gui, v *gocui.View) error {
	fileList.widget.previousPage()
	return nil
}}

var gotoFileCommand = commandType{Name: "Transfer file", f: func(g *gocui.Gui, v *gocui.View) error {
	fileItem := fileList.widget.items[fileList.widget.selectedItem].(map[string]interface{})
	//filename := item["name"].(string)
	tracelog.Printf("selected file:'%v'", fileItem)
	if fileBrowser.sourceSelection {
		if fileItem["dir"].(bool) {
			setFileListContent(fileItem["name"].(string))
		} else {
			sourceFile = fileBrowser.getPath(fileItem["name"].(string))
			if fileBrowser.local {
				fileBrowser = newRemoteFileBrowser(false, "/")
			} else {
				fileBrowser = newLocalFileBrowser(false, ".")
			}
			setFileListContent("")
		}
	} else {
		if fileItem["dir"].(bool) {
			setFileListContent(fileItem["name"].(string))
		} else {
			transferFile(fileBrowser.getPath(""))
		}
	}

	return nil
}}

var nextContainerFiletransferCommand = commandType{Name: "Next container", f: func(g *gocui.Gui, v *gocui.View) error {
	nextFileTransferContainer()
	return nil
}}

func bindKey(g *gocui.Gui, show bool, keyBind keyEventType, command commandType) {
	if err := g.SetKeybinding(keyBind.Viewname, keyBind.Key, keyBind.mod, command.f); err != nil {
		errorlog.Panicln(err)
	}
	kb := keyBindingType{show, keyBind, command}
	keyBindings = append(keyBindings, kb)
}

func filterKeyBindings(filter func(kb keyBindingType) bool) []keyBindingType {
	result := []keyBindingType{}
	for _, k := range keyBindings {
		if filter(k) {
			result = append(result, k)
		}
	}
	return result
}

func keyBindingsForView(viewName string) string {
	kbs := filterKeyBindings(func(kb keyBindingType) bool {
		return viewName == kb.KeyEvent.Viewname && kb.show
	})
	res := ""
	for _, kb := range kbs {
		res = res + "," + kbToText(kb)
	}
	return res
}

func kbToText(kb keyBindingType) string {
	k := kb.KeyEvent
	cmd := kb.Command.Name
	return fmt.Sprintf("%s %s", keyString(k), cmd)
}
