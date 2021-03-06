package kubexp

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alitari/gocui"
)

/* local variables naming conventions:
<type>[<collections>][<context>]

types:
resource: re

response: rp
resource item: ri
namespace: ns
error: err

collections:
array/slice: s

context:
selected: sel

*/

type resourceCategoryType struct {
	name      string
	resources []interface{}
}

type stateType struct {
	name      string
	enterFunc func(fromState stateType)
	exitFunc  func(toState stateType)
}

var fileBrowser *fileBrowserType

var contextColorFunc = func(item interface{}) gocui.Attribute {
	context := cfg.contexts[clusterList.widget.selectedItem]
	return strToColor(context.color)
}

var clusterChangeFooter = "*c*=change"
var namespaceChangeFooter = "*n*=change"
var categoryChangeFooter = "*r*=change category"
var menuSelectFooter = "*←*,*→*=select"
var listSelectFooter = "*↑*,*↓*=select"
var pageSelectFooter = "*⭻*,*⭽*=select Page"
var scrollLineFooter = "*↑*,*↓*=scroll up/down"
var scrollPageFooter = "*⭻*,*⭽*=page previous/next"
var scrollLeftRightFooter = "*Ctrl-a*=Scroll left *Ctrl-d*=Scroll right"
var detailsViewFooter = "*RETURN*=show details"
var backToListFooter = "*RETURN*=back to list"
var setSelectionFooter = "*RETURN*=set"
var setFileSelectionFooter = "*RETURN*=step in or set"
var exitFooter = "*Ctrl-c*=exit"
var delResourceFooter = "*DELETE*=delete resource"
var podsFooter = "*u*=upload *d*=download *1-6*=exec container *p*=port forward"
var scaleFooter = "*+*,*-*=scale up/down"
var changeContainerFooter = "*Ctrl-o*=change container"
var reloadFooter = "*SPACE*=reload"
var helpFooter = "*h*=help"

var currentState stateType

func setState(state stateType) {
	if state.name == currentState.name {
		return
	}
	currentState.exitFunc(state)
	state.enterFunc(currentState)
	currentState = state
}

var initState = stateType{
	name: "initState",
	enterFunc: func(fromState stateType) {

	},
	exitFunc: func(toState stateType) {
		clusterList.widget.items = toIfc(cfg.contexts)
		// contextColor := strToColor(cfg.contexts[0].color)
		// g.FrameFgColor = contextColor
		// g.FrameBgColor = gocui.ColorBlack

		err := backend.createWatches(cfg.resources)
		if err != nil {
			errorlog.Panicf("Can't connect to api server. url:%s, error: %v", backend.context.Cluster.URL, err)
		}
		resourceMenu.widget.items = resources()
		clusterRes := clusterRes()
		clusterResourcesWidget.setContent(clusterRes, tpl("clusterResources", clusterResourcesTemplate))
	},
}

var selectNsState = stateType{
	name: "selectNsState",
	enterFunc: func(fromState stateType) {
		namespaceList.widget.focus = true
		namespaceList.widget.footer = setSelectionFooter + " " + listSelectFooter
	},
	exitFunc: func(toState stateType) {
		namespaceList.widget.focus = false
	},
}

var selectContextState = stateType{
	name: "selectContextState",
	enterFunc: func(fromState stateType) {
		clusterList.widget.focus = true
		clusterList.widget.footer = setSelectionFooter + " " + listSelectFooter + " " + exitFooter
	},
	exitFunc: func(toState stateType) {
		clusterList.widget.focus = false
	},
}

var browseState = stateType{
	name: "browseState",
	enterFunc: func(fromState stateType) {
		if fromState.name == initState.name {
			ns := selectedNamespace()
			selRes := selectedResource()
			updateResourceItemsListTitle(selRes.Name)
			ris := backend.resourceItems(ns, selRes)
			resourceItemsList.widget.items = ris
			resourceItemsList.widget.template = resourceListTpl(selRes)
		}
		resourceMenu.widget.visible = true
		resourceItemsList.widget.visible = true
		resourceItemsList.widget.focus = true
		clusterList.widget.footer = clusterChangeFooter
		namespaceList.widget.footer = namespaceChangeFooter
		resourceMenu.widget.footer = menuSelectFooter + " " + categoryChangeFooter
		updateResourceItemsListFooter()

	},
	exitFunc: func(fromState stateType) {
		resourceItemsList.widget.focus = false
		resourceItemsList.widget.visible = false
		resourceMenu.widget.visible = false
		clusterList.widget.footer = ""
		namespaceList.widget.footer = ""

	},
}

var detailState = stateType{
	name: "detailState",
	enterFunc: func(fromState stateType) {
		resourcesItemDetailsMenu.widget.items = resourceItemDetailsViews()
		resourcesItemDetailsMenu.widget.selectedItem = 0
		setResourceItemDetailsPart()
		resourceItemDetailsWidget.visible = true
		resourcesItemDetailsMenu.widget.visible = true
		searchmodeWidget.visible = true
		searchmodeWidget.active = true
		resourcesItemDetailsMenu.widget.footer = menuSelectFooter
		resourceItemDetailsWidget.footer = scrollLineFooter + " " + scrollLeftRightFooter + " " + scrollPageFooter + " " + backToListFooter
	},
	exitFunc: func(fromState stateType) {
		resourceItemDetailsWidget.visible = false
		searchmodeWidget.visible = false
		resourcesItemDetailsMenu.widget.visible = false
		backend.closePodLogsWatch()
	},
}

var helpState = stateType{
	name: "helpState",
	enterFunc: func(fromState stateType) {
		helpWidget.active = true
		helpWidget.visible = true
		helpWidget.setContent(keyBindings, tpl("help", helpTemplate))
		helpWidget.footer = backToListFooter + " " + scrollLineFooter
	},
	exitFunc: func(fromState stateType) {
		helpWidget.active = false
		helpWidget.visible = false
	},
}

var fileState = stateType{
	name: "fileState",
	enterFunc: func(fromState stateType) {
		fileList.widget.visible = true
		fileList.widget.focus = true
		details := resourceItemsList.widget.items[resourceItemsList.widget.selectedItem]
		selectedContainerIndex = 0
		containerNames = resItemContainers(details)
		if len(containerNames) > 1 {
			fileList.widget.footer = changeContainerFooter + " " + setFileSelectionFooter + " " + listSelectFooter + " " + exitFooter
		} else {
			fileList.widget.footer = setFileSelectionFooter + " " + listSelectFooter + " " + exitFooter
		}
	},
	exitFunc: func(fromState stateType) {
		fileList.widget.visible = false
		fileList.widget.focus = false
	},
}

var execPodState = stateType{
	name: "execPodState",
	enterFunc: func(fromState stateType) {
		execWidget.visible = true
	},
	exitFunc: func(fromState stateType) {
		execWidget.active = false
		execWidget.visible = false
	},
}

var errorState = stateType{
	name: "errorState",
	enterFunc: func(fromState stateType) {
		errorWidget.active = true
		errorWidget.visible = true

	},
	exitFunc: func(fromState stateType) {
		errorWidget.active = false
		errorWidget.visible = false
	},
}

var confirmState = stateType{
	name: "confirmState",
	enterFunc: func(fromState stateType) {
		confirmWidget.active = true
		confirmWidget.visible = true

	},
	exitFunc: func(fromState stateType) {
		confirmWidget.active = false
		confirmWidget.visible = false
	},
}

var loadingState = stateType{
	name: "loadingState",
	enterFunc: func(fromState stateType) {
		loadingWidget.active = true
		loadingWidget.visible = true

	},
	exitFunc: func(fromState stateType) {
		loadingWidget.active = false
		loadingWidget.visible = false
	},
}

var namespaceALL = map[string]interface{}{"metadata": map[string]interface{}{"name": "*ALL*"}}

var confirmCommand commandType

var portforwardProxies = map[string][]*portforwardProxy{}

var portforwardStartPort int
var currentPortforwardPort int
var restCallTimeout int
var kubeCtlTimeout int
var clusterLivenessPeriod int

var clusterList *nlist
var clusterResourcesWidget *textWidget
var namespaceList *nlist
var resourceMenu *nmenu
var resourcesItemDetailsMenu *nmenu
var searchmodeWidget *searchWidget

var resourceItemsList *nlist
var resourceItemDetailsWidget *textWidget
var helpWidget *textWidget
var execWidget *shellWidget
var errorWidget *textWidget
var confirmWidget *textWidget
var loadingWidget *textWidget
var fileList *nlist

var selectedResourceCategoryIndex = 0
var selectedClusterInfoIndex = 0

var maxX, maxY int

var cfg *configType
var backend *backendType

var resourceCategories []string

var containerNames []string
var selectedContainerIndex int

var sourceFile string

var g *gocui.Gui

var logLevel *string
var logFilePath *string

var wg sync.WaitGroup
var leaveApp = false
var exe = make(chan *exec.Cmd)

var keyBindings = []keyBindingType{}

// Run entrypoint of the program
func Run() {
	parseFlags()
	currentPortforwardPort = portforwardStartPort

	if len(*logFilePath) != 0 {
		logFile, err := os.OpenFile(*logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		defer logFile.Close()
		if err != nil {
			initLog(ioutil.Discard)
			warnStderrlog.Printf("can't open log file '%s', no log file written!", *logFilePath)
			time.Sleep(3000 * time.Millisecond)
		} else {
			initLog(logFile)
		}
	} else {
		initLog(ioutil.Discard)
	}
	infolog.Printf("-------------------------------------< Startup >---------------------------------------------\n")
	cfg = newConfig(*configFile)
	ctx := cfg.contexts[0]
	resourceCategories = cfg.allResourceCategories()
	backend = newBackend(ctx)
	go commandRunner()
	currentState = initState

	for {
		wg.Add(1)
		initGui()

		if leaveApp {
			g.Close()
			break
		}
		wg.Wait()
	}

}

func commandRunner() {
	for {
		select {
		case cmd := <-exe:
			unbindKeys()
			g.Close()

			cmd.Stdin = os.Stdin
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout
			cmd.Run()
			wg.Done()
		}
	}
}

func initGui() {
	var err error
	g, err = gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		errorlog.Panicln(err)
	}
	if currentState.name != browseState.name {
		createWidgets()
	}
	g.SetManager(clusterList.widget, clusterResourcesWidget, namespaceList.widget, resourceMenu.widget, resourcesItemDetailsMenu.widget, searchmodeWidget, resourceItemsList.widget, resourceItemDetailsWidget, helpWidget, errorWidget, execWidget, confirmWidget, loadingWidget, fileList.widget)

	bindKeys()
	if currentState.name != browseState.name {
		namespaceList.widget.visible = false
		setState(browseState)
		if cfg.isNew {
			setState(helpState)
		}
	} else {
		newResourceCategory()
	}

	ctx := cfg.contexts[clusterList.widget.selectedItem]
	contextColor := strToColor(ctx.color)
	g.FrameFgColor = contextColor
	g.FrameBgColor = gocui.ColorBlack

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		errorlog.Printf("error in mail loop: %v", err)
	}
}

func parseFlags() {
	configFile = flag.String("config", filepath.Join(homeDir(), ".kube", "config"), "absolute path to the config file")
	logLevel = flag.String("logLevel", "info", "verbosity of log output. Values: 'trace','info','warn','error'")
	logFilePath = flag.String("logFile", "./kubexp.log", "fullpath to log file, set empty ( -logFile='') if no logfile should be used")
	flag.IntVar(&portforwardStartPort, "portForwardStartPort", 32100, "start of portforward range")
	flag.IntVar(&restCallTimeout, "restCallTimeout", 3, "time out for rest calls in seconds")
	flag.IntVar(&kubeCtlTimeout, "kubectlTimeout", 5, "time out for kubectl calls in seconds")

	flag.IntVar(&clusterLivenessPeriod, "clusterLivenessPeriod", 5, "cluster liveness check period in seconds")

	flag.Parse()
}

func selectedNamespace() string {
	return resItemName(namespaceList.widget.items[namespaceList.widget.selectedItem])
}

func selectedResource() resourceType {
	return resourceMenu.widget.items[resourceMenu.widget.selectedItem].(resourceType)
}

func selectedResourceItemDetailsView() viewType {
	if len(resourcesItemDetailsMenu.widget.items) > resourcesItemDetailsMenu.widget.selectedItem {
		return resourcesItemDetailsMenu.widget.items[resourcesItemDetailsMenu.widget.selectedItem].(viewType)
	}
	return resourcesItemDetailsMenu.widget.items[0].(viewType)
}

func selectedResourceItemName() string {
	if len(resourceItemsList.widget.items) > 0 {
		return resItemName(resourceItemsList.widget.items[resourceItemsList.widget.selectedItem])
	}
	return ""
}

func selectedResourceItemNamespace() string {
	if len(resourceItemsList.widget.items) > 0 {
		return resItemNamespace(resourceItemsList.widget.items[resourceItemsList.widget.selectedItem])
	}
	return ""
}

func updateResourceItemDetailPart() {
	if g == nil {
		return
	}
	g.Update(func(gui *gocui.Gui) error {
		reloadResourceItemDetailsPart()
		return nil
	})
}

func updateResourceItemList(reset bool) {
	if g == nil {
		return
	}
	g.Update(func(gui *gocui.Gui) error {
		if reset {
			newResource()
		} else {
			res := selectedResource()
			resourceItemsList.widget.items = backend.resourceItems(selectedNamespace(), res)
			updateResourceItemsListTitle(res.Name)
		}
		return nil
	})
}

func updateNamespaces() {
	tracelog.Printf("update namespace")
	g.Update(func(gui *gocui.Gui) error {
		nsType := cfg.resourcesOfName("namespaces")
		ris := backend.resourceItems("", nsType)
		namespaceList.widget.items = []interface{}{namespaceALL}
		namespaceList.widget.items = append(namespaceList.widget.items, ris...)
		return nil
	})
}

func createWidgets() {
	maxX, maxY = g.Size()
	sepXAt := int(float64(maxX) * 0.75)
	sepXAt2 := int(float64(maxX) * 0.3)
	sepYAt := 7

	g.Highlight = false
	g.SelFgColor = gocui.ColorRed
	g.SelBgColor = gocui.ColorGreen

	clusterList = newNlist("cluster", 1, 1, sepXAt2, 6)
	clusterList.widget.expandable = true
	clusterList.widget.title = "Cluster"
	clusterList.widget.visible = true
	clusterList.widget.frame = true
	clusterList.widget.template = tpl("clusterTemplate", `{{ "Name:" | contextColorEmp }} {{ .Name | printf "%-20.20s" }}  {{ "URL:" | contextColorEmp }} {{ .Cluster.URL }}`)
	clusterResourcesWidget = newTextWidget("clusterResources", "cluster resources", true, false, sepXAt2+2, 1, sepXAt-sepXAt2-1, 2)

	namespaceList = newNlist("namespaces", sepXAt+2, 1, maxX-sepXAt-3, 10)
	namespaceList.widget.expandable = true
	namespaceList.widget.title = "Namespace"
	namespaceList.widget.visible = true
	namespaceList.widget.frame = true
	namespaceList.widget.template = tpl("namespace", `{{.metadata.name | printf "%-50.50s" }}`)
	namespaceList.widget.items = []interface{}{namespaceALL}

	resourceMenu = newNmenu("resourcesMenu", 1, 4, maxX-2, 16)
	resourceMenu.widget.visible = true
	resourceMenu.widget.title = fmt.Sprintf("Resources - %s", resourceCategories[selectedResourceCategoryIndex])
	resourceMenu.widget.frame = true
	resourceMenu.widget.template = tpl("resource", `{{ .ShortName }}`)

	resourcesItemDetailsMenu = newNmenu("resourcesItemDetailsMenu", 1, 4, sepXAt, 16)
	resourcesItemDetailsMenu.widget.visible = false
	resourcesItemDetailsMenu.widget.frame = true
	resourcesItemDetailsMenu.widget.template = tpl("resourcesItemDetailsMenu", `{{ .Name }}`)

	//resourceRenderer
	searchmodeWidget = newSearchWidget("search", "search", false, sepXAt+2, 4, maxX-sepXAt-3)

	resourceItemsList = newNlist("resourceItems", 1, sepYAt, maxX-2, maxY-sepYAt-1)
	resourceItemsList.widget.visible = true
	resourceItemsList.widget.title = "Resource Items "
	resourceItemsList.widget.frame = true
	resourceItemsList.widget.headerItem = map[string]interface{}{"header": "true"}
	resourceItemsList.widget.headerFgColor = gocui.ColorDefault | gocui.AttrBold

	resourceItemDetailsWidget = newTextWidget("text", "resource item details", false, true, 1, sepYAt, maxX-2, maxY-sepYAt-1)

	helpWidget = newTextWidget("help", "HELP", false, false, maxX/2-32, 5, 64, maxY-10)

	execWidget = newShellWidget("exec", "execWidget", 2, 5, maxX-4, maxY-10)

	errorWidget = newTextWidget("error", "ERROR", false, false, 10, 7, maxX-20, 10)
	errorWidget.wrap = true

	confirmWidget = newTextWidget("confirm", "Confirm", false, false, 20, 7, maxX-40, 4)

	loadingWidget = newTextWidget("loading", "", false, false, 30, 10, maxX-60, 4)

	fileList = newNlist("files", maxX/2-45, 5, 90, maxY-10)
	fileList.widget.expandable = false
	fileList.widget.visible = false
	fileList.widget.frame = true
	fileList.widget.headerItem = map[string]interface{}{"header": "true"}
	fileList.widget.template = tpl("files", `
	{{- header "Mode" . .mode | printf "  %-12.12s  " -}}
	{{- header "Size" . .size | printf "%10s  " -}}
	{{- header "Time" . .time | printf "%-16.16s  " -}}
	{{- header "Name" . .name | printf "%-40.40s" -}}`)
	fileList.widget.headerFgColor = gocui.ColorDefault | gocui.AttrBold
}

func updateResourceItemsListFooter() {
	resourceItemsList.widget.footer = delResourceFooter + " " + listSelectFooter + " " + detailsViewFooter + " " + reloadFooter + " " + helpFooter + " " + exitFooter
	if resourceItemsList.widget.pc() > 1 {
		resourceItemsList.widget.footer = pageSelectFooter + " " + resourceItemsList.widget.footer
	}
	switch selectedResource().Name {
	case "pods":
		resourceItemsList.widget.footer = podsFooter + " " + resourceItemsList.widget.footer
	case "deployments":
		fallthrough
	case "replicationcontrollers":
		fallthrough
	case "replicasets":
		fallthrough
	case "daemonsets":
		fallthrough
	case "statefulsets":
		resourceItemsList.widget.footer = scaleFooter + " " + resourceItemsList.widget.footer
	}
}

func resourceItemDetailsViews() []interface{} {
	res := selectedResource()
	ret := make([]interface{}, 0)
	for _, v := range cfg.detailViews(res) {
		ret = append(ret, v)
	}
	return ret
}

func filterResources(res []resourceType) []interface{} {
	ret := make([]interface{}, 0)
	for _, r := range res {
		ns := selectedNamespace()
		resItems := backend.resourceItems(ns, r)

		if len(resItems) > 0 {
			ret = append(ret, r)
		} else {
			tracelog.Printf("no resourceItems for %s", r.Name)
		}
	}
	return ret
}

func showError(mess string, err error) {
	mess = strings.Join([]string{mess, fmt.Sprintf(".See log file '%s'", *logFilePath)}, "")
	g.Update(func(gui *gocui.Gui) error {
		co := []interface{}{mess, err}
		errorWidget.setContent(co, tpl("error", errorTemplate))
		setState(errorState)
		return nil
	})
}

func showConfirm(mess string, command commandType) {
	confirmCommand = command
	g.Update(func(gui *gocui.Gui) error {
		co := []interface{}{mess}
		confirmWidget.setContent(co, tpl("confirm", confirmTemplate))
		setState(confirmState)
		return nil
	})
}

func showLoading(command commandType, g *gocui.Gui, v *gocui.View) {
	co := []interface{}{"Loading..."}
	loadingWidget.setContent(co, tpl("loading", loadingTemplate))
	setState(loadingState)
	g.Update(func(gui *gocui.Gui) error {
		command.f(g, v)
		setState(browseState)
		return nil
	})
}

func resources() []interface{} {
	c := resourceCategories[selectedResourceCategoryIndex]
	rc := cfg.resourcesOfCategory(c)
	return filterResources(rc)
}

func findResourceCategoryWithResources(offset int) {
	for len(resources()) == 0 {
		nextResourceCategory(offset)
	}
	newResourceCategory()
}

func nextResourceCategory(offset int) {
	selectedResourceCategoryIndex += offset
	switch {
	case selectedResourceCategoryIndex > len(resourceCategories)-1:
		selectedResourceCategoryIndex = 0
	case selectedResourceCategoryIndex < 0:
		selectedResourceCategoryIndex = len(resourceCategories) - 1
	}
}

func newResourceCategory() {
	resCat := resourceCategories[selectedResourceCategoryIndex]
	resourceMenu.widget.title = fmt.Sprintf("Resources - %s", resCat)
	namespaceList.widget.visible = resCat != "cluster/metadata"

	resourceMenu.widget.selectedItem = 0
	resourceMenu.widget.items = resources()

	selRes := selectedResource()
	ns := selectedNamespace()
	resItems := backend.resourceItems(ns, selRes)

	resourceItemsList.widget.items = resItems
	updateResourceItemsListTitle(selRes.Name)
	updateResourceItemsListFooter()
	resourceItemsList.widget.template = resourceListTpl(selRes)
	if resourceItemsList.widget.selectedItem >= len(resourceItemsList.widget.items) {
		resourceItemsList.widget.selectedItem = 0
	}
}

func newContext() error {
	ctx := cfg.contexts[clusterList.widget.selectedItem]
	backend.closeWatches()
	backend = newBackend(ctx)

	contextColor := strToColor(ctx.color)
	g.FrameFgColor = contextColor
	g.FrameBgColor = gocui.ColorBlack

	err := backend.createWatches(cfg.resources)
	if err != nil {
		showError(fmt.Sprintf("Can't connect to api server, url:%s ", backend.context.Cluster.URL), err)
	}
	res := cfg.resourcesOfName("namespaces")
	resItems := backend.resourceItems("", res)

	namespaceList.widget.items = resItems
	namespaceList.widget.selectedItem = 0
	clusterRes := clusterRes()
	clusterResourcesWidget.setContent(&clusterRes, tpl("clusterResources", clusterResourcesTemplate))
	findResourceCategoryWithResources(1)
	return nil
}

func retrieveContextToken(ctx *contextType) error {
	cmd := kubectl(ctx.Name, "get", "secret")
	out, errorStr, err := runCmd(cmd)
	if err != nil {
		return fmt.Errorf("Error running command '%v': %s\n Details: %s", cmd.Args, err.Error(), errorStr)
	}
	var tokenSecretName string
	for _, l := range strings.Split(string(out), "\n") {
		name := strings.Split(l, " ")[0]
		if strings.HasPrefix(name, "default-token") {
			tokenSecretName = name
			break
		}
	}

	cmd = kubectl(ctx.Name, "get", "secret", tokenSecretName, "-o", "json")
	out, errorStr, err = runCmd(cmd)
	if err != nil {
		return fmt.Errorf("Error running command '%v': %v\n Details: %s", cmd.Args, err, errorStr)
	}

	tokenSecret := unmarshall(out)
	tokenB64 := val1(tokenSecret, "{{.data.token}}")
	token, err := base64.StdEncoding.DecodeString(tokenB64)
	if err != nil {
		return err
	}
	tokenStr := string(token)
	tracelog.Printf(" retrieved token for context '%s'...", ctx.Name)
	ctx.user.token = tokenStr
	return nil
}

func startFiletransfer(isUpload bool) {
	if selectedResource().Name == "pods" {
		setState(fileState)
		if isUpload {
			fileBrowser = newLocalFileBrowser(true, ".")
		} else {
			fileBrowser = newRemoteFileBrowser(true, "/")
		}
		setFileListContent("")
	}
}

func transferFile(destPath string) {
	podName := selectedResourceItemName()
	ns := selectedResourceItemNamespace()
	con := containerNames[selectedContainerIndex]

	var cmd *exec.Cmd
	var mess string

	if fileBrowser.local {
		absPath, _ := filepath.Abs(destPath)
		cmd = kubectl(backend.context.Name, "-n", ns, "cp", "-c", con, podName+":"+sourceFile, absPath)
		mess = fmt.Sprintf("file download:'%s'\n in pod '%s' from namespace '%s'\n to local dir '%s' ", sourceFile, podName, ns, destPath)
	} else {
		cmd = kubectl(backend.context.Name, "-n", ns, "cp", "-c", con, sourceFile, podName+":"+path.Join(destPath, path.Base(sourceFile)))
		mess = fmt.Sprintf("file upload:'%s' \n to '%s' in pod '%s' in namespace '%s'", sourceFile, destPath, podName, ns)
	}

	co := []interface{}{mess + "..."}
	loadingWidget.setContent(co, tpl("loading", loadingTemplate))
	setState(loadingState)
	g.Update(func(gui *gocui.Gui) error {

		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			fullmess := fmt.Sprintf("Problem: %s \n details: %s", mess, fmt.Sprintf(fmt.Sprint(err)+": %s ", stderr.String()))
			errorlog.Print(fullmess)
			showError(fullmess, err)
			return nil
		}
		infolog.Print(mess + " succeeded!")
		setState(browseState)
		return nil
	})
}

func strToColor(colorStr string) gocui.Attribute {
	switch colorStr {
	case "Blue":
		return gocui.ColorBlue
	case "Magenta":
		return gocui.ColorMagenta
	case "White":
		return gocui.ColorWhite
	case "Cyan":
		return gocui.ColorCyan
	}
	return gocui.ColorDefault
}

func strToInlineColor(colorStr string, emp bool) inlineColorType {
	switch colorStr {
	case "Blue":
		if emp {
			return blueEmpInlineColor
		}
		return blueInlineColor

	case "Magenta":
		if emp {
			return magentaEmpInlineColor
		}
		return magentaInlineColor

	case "White":
		if emp {
			return whiteEmpInlineColor
		}
		return whiteInlineColor

	case "Cyan":
		if emp {
			return cyanEmpInlineColor
		}
		return cyanInlineColor
	}
	return whiteInlineColor
}

func nameSorting() {
	if backend.sorter.getName() == "NameSorter" {
		backend.sorter.setAscending(!backend.sorter.getAscending())
	} else {
		backend.sorter = &nameSorterType{ascending: true}
	}
}

func ageSorting() {
	if backend.sorter.getName() == "AgeSorter" {
		backend.sorter.setAscending(!backend.sorter.getAscending())
	} else {
		backend.sorter = &timeSorterType{ascending: true}
	}
}

func deleteResource(noGracePeriod bool) {
	ns := selectedResourceItemNamespace()
	res := selectedResource()
	rname := selectedResourceItemName()
	_, err := backend.delete(ns, res, rname, noGracePeriod)
	if err != nil {
		showError(fmt.Sprintf("Can't delete %s on namespace %s with name '%s' ", res.Name, ns, rname), err)
	}
}

func scaleResource(replicas int) {
	res := selectedResource()
	rname := selectedResourceItemName()
	ns := selectedResourceItemNamespace()
	resDetails := resourceItemsList.widget.items[resourceItemsList.widget.selectedItem]
	_, err := backend.scale(ns, res, rname, resDetails, replicas)
	if err != nil {
		showError(fmt.Sprintf("Can't scale %s on namespace %s with name '%s' ", res.Name, ns, rname), err)
	}
}

func newResource() {
	selRes := selectedResource()
	selNs := selectedNamespace()

	resItems := backend.resourceItems(selNs, selRes)
	updateResourceItemsListTitle(selRes.Name)
	resourceItemsList.widget.items = resItems
	resourceItemsList.widget.template = resourceListTpl(selRes)
	resourceItemsList.widget.selectedPage = 0
	resourceItemsList.widget.selectedItem = 0
	updateResourceItemsListFooter()
}

func updateResourceItemsListTitle(resourceItemName string) {
	var titleTmp string
	if backend.watches[resourceItemName].online {
		titleTmp = fmt.Sprintf("  %-30.30s ", resourceItemName)
		resourceItemsList.widget.tableFgColor = gocui.ColorDefault
	} else {
		titleTmp = fmt.Sprintf("  %-30.30s  **OFFLINE**", resourceItemName)
		resourceItemsList.widget.tableFgColor = gocui.ColorRed
	}
	resourceItemsList.widget.title = titleTmp
}

func leaveResourceItemDetailsPart() {
	view := selectedResourceItemDetailsView()
	if view.Name == "logs" {
		backend.closePodLogsWatch()
		selectedContainerIndex = 0
	}
}

func nextLogContainer() {
	l := len(containerNames)
	if l <= 0 || selectedResourceItemDetailsView().Name != "logs" {
		return
	}
	if selectedContainerIndex < l-1 {
		selectedContainerIndex = selectedContainerIndex + 1
	} else {
		selectedContainerIndex = 0
	}
	backend.closePodLogsWatch()
	setResourceItemDetailsPart()
}

func nextFileTransferContainer() {
	l := len(containerNames)
	if l <= 0 || fileBrowser.local {
		return
	}
	if selectedContainerIndex < l-1 {
		selectedContainerIndex = selectedContainerIndex + 1
	} else {
		selectedContainerIndex = 0
	}
	fileBrowser = newRemoteFileBrowser(fileBrowser.sourceSelection, "/")
	setFileListContent("")
}

func setFileListContent(dir string) {
	fileList.widget.items = fileBrowser.getFileList(dir)
	fileList.widget.title = fileBrowser.getContext()
	fileList.widget.selectedItem = 0
	fileList.widget.selectedPage = 0
}

func setResourceItemDetailsPart() {
	rname := selectedResourceItemName()
	view := selectedResourceItemDetailsView()
	resourceItemDetailsWidget.xOffset = 0
	resourceItemDetailsWidget.yOffset = 0
	details := resourceItemsList.widget.items[resourceItemsList.widget.selectedItem]

	if view.Name == "logs" {
		containerNames = resItemContainers(details)
		backend.watchPodLogs(resItemNamespace(details), rname, containerNames[selectedContainerIndex])
		tracelog.Printf("containerNames: %v", containerNames)
		if len(containerNames) > 1 {
			resourceItemDetailsWidget.footer = changeContainerFooter + " " + scrollLineFooter + " " + scrollLeftRightFooter + " " + scrollPageFooter + " " + backToListFooter
		} else {
			resourceItemDetailsWidget.footer = scrollLineFooter + " " + scrollLeftRightFooter + " " + scrollPageFooter + " " + backToListFooter
		}
	} else {
		resourceItemDetailsWidget.footer = scrollLineFooter + " " + scrollLeftRightFooter + " " + scrollPageFooter + " " + backToListFooter
	}
	reloadResourceItemDetailsPart()
}

func reloadResourceItemDetailsPart() {
	res := selectedResource()
	rname := selectedResourceItemName()
	view := selectedResourceItemDetailsView()
	details := resourceItemsList.widget.items[resourceItemsList.widget.selectedItem]
	resourceItemDetailsWidget.setContent(details, resourceTpl(res, view))
	if view.Name == "logs" {
		resourceItemDetailsWidget.title = fmt.Sprintf("%s - %s  details, container: %s ", res.Name, rname, containerNames[selectedContainerIndex])
	} else {
		resourceItemDetailsWidget.title = fmt.Sprintf("%s - %s  details ", res.Name, rname)
	}
}

func toggleDetailBrowseState() {
	if currentState.name == browseState.name {
		setState(detailState)
		return
	}
	setState(browseState)
}

func quitResourceItemDetails() {
	resourceItemDetailsWidget.visible = false
}

func findInResourceItemDetails(text string) bool {
	return resourceItemDetailsWidget.find(text)
}

func createPortforwardProxy(ns, podName string, pm portMapping) error {
	p := newPortforwardProxy(ns, podName, pm)
	k := p.namespace + "/" + p.pod
	if portforwardProxies[k] == nil {
		portforwardProxies[k] = make([]*portforwardProxy, 0)
	}
	portforwardProxies[k] = append(portforwardProxies[k], p)
	return p.execute()
}

func removeAllPortforwardProxies() error {
	for k, pf := range portforwardProxies {
		err := removePortforwardProxy(k)
		if err != nil {
			errorlog.Printf("can't remove portforwardProxy: %v\n error: %v", pf, err)
		}
	}
	portforwardProxies = map[string][]*portforwardProxy{}
	return nil
}

func removePortforwardProxyofPod(ns, podName string) error {
	k := ns + "/" + podName
	return removePortforwardProxy(k)
}

func removePortforwardProxy(key string) error {
	pl := portforwardProxies[key]
	if pl == nil {
		return fmt.Errorf("proxies for '%s' do not exist", key)
	}
	for _, p := range pl {
		err := p.stop()
		if err != nil {
			return err
		}
	}
	delete(portforwardProxies, key)
	return nil
}

func unbindKeys() {
	for _, kb := range keyBindings {
		if err := g.DeleteKeybinding(kb.KeyEvent.Viewname, kb.KeyEvent.Key, kb.KeyEvent.mod); err != nil {
			errorlog.Panicln(err)
		}
	}
	keyBindings = []keyBindingType{}
}

func bindKeys() {
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'h', mod: gocui.ModNone}, showHelpCommand)
	bindKey(g, false, keyEventType{Viewname: helpWidget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, quitWidgetCommand)
	bindKey(g, false, keyEventType{Viewname: helpWidget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, scrollDownHelpCommand)
	bindKey(g, false, keyEventType{Viewname: helpWidget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, scrollUpHelpCommand)

	bindKey(g, false, keyEventType{Viewname: errorWidget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, quitWidgetCommand)

	bindKey(g, false, keyEventType{Viewname: confirmWidget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, quitWidgetCommand)
	bindKey(g, false, keyEventType{Viewname: confirmWidget.name, Key: 'n', mod: gocui.ModNone}, quitWidgetCommand)
	bindKey(g, false, keyEventType{Viewname: confirmWidget.name, Key: 'y', mod: gocui.ModNone}, executeConfirmCommand)

	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyCtrlC, mod: gocui.ModNone}, quitCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowRight, mod: gocui.ModNone}, nextResourceCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowLeft, mod: gocui.ModNone}, previousResourceCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, nextLineCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousLineCommand)

	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'c', mod: gocui.ModNone}, gotoSelectContextStateCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeySpace, mod: gocui.ModNone}, loadContextCommand)
	// bindKey(g,false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'C', mod: gocui.ModNone}, previousContextCommand)
	// bindKey(g,false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'f', mod: gocui.ModNone}, nextNamespaceCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'n', mod: gocui.ModNone}, gotoSelectNamespaceStateCommand)
	// bindKey(g,false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'F', mod: gocui.ModNone}, previousNamespaceCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, toggleResourceItemDetailsCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'r', mod: gocui.ModNone}, nextResourceCategoryCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, nextResourceItemListPageCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, previousResourceItemListPageCommand)

	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyDelete, mod: gocui.ModNone}, deleteConfirmCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyDelete, mod: gocui.ModAlt}, deleteNoGracePeriodConfirmCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '+', mod: gocui.ModNone}, scaleUpCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '-', mod: gocui.ModNone}, scaleDownCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'm', mod: gocui.ModNone}, nameSortCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'a', mod: gocui.ModNone}, ageSortCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'x', mod: gocui.ModNone}, execShellCommand0)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '1', mod: gocui.ModNone}, execShellCommand0)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '2', mod: gocui.ModNone}, execShellCommand1)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '3', mod: gocui.ModNone}, execShellCommand2)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'X', mod: gocui.ModNone}, execBashCommand0)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '4', mod: gocui.ModNone}, execBashCommand0)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '5', mod: gocui.ModNone}, execBashCommand1)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: '6', mod: gocui.ModNone}, execBashCommand2)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'p', mod: gocui.ModNone}, portForwardSamePortCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'P', mod: gocui.ModNone}, portForwardCommand)

	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowRight, mod: gocui.ModNone}, nextResourceItemDetailPartCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowLeft, mod: gocui.ModNone}, previousResourceItemDetailPartCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeySpace, mod: gocui.ModNone}, reloadResourceItemDetailPartCommand)

	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlO, mod: gocui.ModNone}, nextContainerCommand)

	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, scrollDownCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, scrollUpCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlA, mod: gocui.ModNone}, scrollLeftCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlD, mod: gocui.ModNone}, scrollRightCommand)

	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, pageDownCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, pageUpCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyHome, mod: gocui.ModNone}, homeCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyEnd, mod: gocui.ModNone}, endCommand)

	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, toggleResourceItemDetailsCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlN, mod: gocui.ModNone}, findNextCommand)
	bindKey(g, false, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlP, mod: gocui.ModNone}, findPreviousCommand)

	bindKey(g, false, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, selectNamespaceLoadingCommand)
	bindKey(g, false, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousNamespaceCommand)
	bindKey(g, false, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, nextNamespaceCommand)
	bindKey(g, false, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, nextNamespacePageCommand)
	bindKey(g, false, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, previousNamespacePageCommand)

	bindKey(g, false, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, loadContextCommand)
	bindKey(g, false, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousContextCommand)
	bindKey(g, false, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, nextContextCommand)
	bindKey(g, false, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, nextContextPageCommand)
	bindKey(g, false, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, previousContextPageCommand)
	bindKey(g, false, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyCtrlC, mod: gocui.ModNone}, quitWidgetCommand)

	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'u', mod: gocui.ModNone}, uploadFileCommand)
	bindKey(g, false, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'd', mod: gocui.ModNone}, downloadFileCommand)
	bindKey(g, false, keyEventType{Viewname: fileList.widget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, gotoFileCommand)
	bindKey(g, false, keyEventType{Viewname: fileList.widget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousFileCommand)
	bindKey(g, false, keyEventType{Viewname: fileList.widget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, nextFileCommand)
	bindKey(g, false, keyEventType{Viewname: fileList.widget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, nextFilePageCommand)
	bindKey(g, false, keyEventType{Viewname: fileList.widget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, previousFilePageCommand)
	bindKey(g, false, keyEventType{Viewname: fileList.widget.name, Key: gocui.KeyCtrlO, mod: gocui.ModNone}, nextContainerFiletransferCommand)
	bindKey(g, false, keyEventType{Viewname: fileList.widget.name, Key: gocui.KeyCtrlC, mod: gocui.ModNone}, quitWidgetCommand)

}

func toIfc(cis []contextType) []interface{} {
	ifcs := make([]interface{}, len(cis))
	for i, ci := range cis {
		ifcs[i] = ci
	}
	return ifcs
}
