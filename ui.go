package kubexp

import (
	"time"
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
import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/jroimartin/gocui"
)

type resourceCategoryType struct {
	name      string
	resources []interface{}
}

type stateType struct {
	name      string
	enterFunc func(fromState stateType)
	exitFunc  func(toState stateType)
}

var contextColorFunc = func(item interface{}) gocui.Attribute {
	context := cfg.contexts[clusterList.widget.selectedItem]
	return strToColor(context.color)
}

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
		backend.resetCache()
		ns := cfg.resourcesOfName("namespaces")
		ris, err := backend.resourceItems("", ns)
		if err != nil {
			errorlog.Printf("can't load namespaces: %v", err)
		}
		namespaceList.widget.items = ris
		resourceMenu.widget.items = resources()
		clusterRes := clusterRes()
		clusterResourcesWidget.setContent(clusterRes, tpl("clusterResources", clusterResourcesTemplate))
	},
}

var selectNsState = stateType{
	name: "selectNsState",
	enterFunc: func(fromState stateType) {
		namespaceList.widget.focus = true
	},
	exitFunc: func(toState stateType) {
		namespaceList.widget.focus = false
	},
}

var selectContextState = stateType{
	name: "selectContextState",
	enterFunc: func(fromState stateType) {
		clusterList.widget.focus = true
	},
	exitFunc: func(toState stateType) {
		clusterList.widget.focus = false
	},
}

var browseState = stateType{
	name: "browseState",
	enterFunc: func(fromState stateType) {
		if fromState.name == initState.name {
			ns := currentNamespace()
			selRes := currentResource()
			resourceItemsList.widget.title = fmt.Sprintf("%s - Items", selRes.Name)
			ris, err := backend.resourceItems(ns, selRes)
			if err != nil {
				showError(fmt.Sprintf("can't load %s of namespace %s ", selRes, ns), err)
			}
			resourceItemsList.widget.items = ris
			resourceItemsList.widget.template = resourceListTpl(selRes)
		}
		resourceMenu.widget.visible = true
		resourceItemsList.widget.visible = true
		resourceItemsList.widget.focus = true
	},
	exitFunc: func(fromState stateType) {
		resourceItemsList.widget.focus = false
		resourceItemsList.widget.visible = false
		resourceMenu.widget.visible = false
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
	},
	exitFunc: func(fromState stateType) {
		resourceItemDetailsWidget.visible = false
		searchmodeWidget.visible = false
		resourcesItemDetailsMenu.widget.visible = false
	},
}

var helpState = stateType{
	name: "helpState",
	enterFunc: func(fromState stateType) {
		helpWidget.active = true
		helpWidget.visible = true
		helpWidget.setContent(keyBindings, tpl("help", helpTemplate))
	},
	exitFunc: func(fromState stateType) {
		helpWidget.active = false
		helpWidget.visible = false
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

var portforwardProxies = map[string][]*portforwardProxy{}

var portforwardStartPort int
var currentPortforwardPort int

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

var selectedResourceCategoryIndex = 0
var selectedClusterInfoIndex = 0

var maxX, maxY int

var cfg *configType
var backend *backendType

var resourceCategories []string
var g *gocui.Gui

var logLevel *string
var logFilePath *string

// Run entrypoint of the program
func Run() {
	parseFlags()
	currentPortforwardPort = portforwardStartPort
	var err error
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
	resourceCategories = cfg.allResourceCategories()
	backend = newRestyBackend(cfg.contexts[0])

	g, err = gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		errorlog.Panicln(err)
	}
	defer g.Close()

	createWidgets()
	bindKeys()
	currentState = initState
	setState(browseState)
	if cfg.isNew {
		setState(helpState)
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		errorlog.Panicln(err)
	}

}

func parseFlags() {
	configFile = flag.String("config", filepath.Join(homeDir(), ".kube", "config"), "absolute path to the config file")
	logLevel = flag.String("logLevel", "info", "verbosity of log output. Values: 'trace','info','warn','error'")
	logFilePath = flag.String("logFile", "./kubexp.log", "fullpath to log file, set empty ( -logFile='') if no logfile should be used")
	flag.IntVar(&portforwardStartPort, "portForwardStartPort", 32100, "start of portforward range")
	flag.Parse()
}

func currentNamespace() string {
	return resourceName(namespaceList.widget.items[namespaceList.widget.selectedItem])
}

func currentResource() resourceType {
	return resourceMenu.widget.items[resourceMenu.widget.selectedItem].(resourceType)
}

func currentResourceItemDetailsView() viewType {
	return resourcesItemDetailsMenu.widget.items[resourcesItemDetailsMenu.widget.selectedItem].(viewType)
}

func currentResourceItemName() string {
	if len(resourceItemsList.widget.items) > 0 {
		return resourceName(resourceItemsList.widget.items[resourceItemsList.widget.selectedItem])
	}
	return ""
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
	clusterList.widget.title = "[C]luster"
	clusterList.widget.visible = true
	clusterList.widget.frame = true
	clusterList.widget.template = tpl("clusterTemplate", `{{ "Name:" | contextColorEmp }} {{ .Name | printf "%-20.20s" }}  {{ "URL:" | contextColorEmp }} {{ .Cluster.URL }}`)

	clusterResourcesWidget = newTextWidget("clusterResources", "cluster resources", true, sepXAt2+2, 1, sepXAt-sepXAt2-1, 2)
	clusterResourcesWidget.showPos = false

	namespaceList = newNlist("namespaces", sepXAt+2, 1, maxX-sepXAt-3, 10)
	namespaceList.widget.expandable = true
	namespaceList.widget.title = "[N]amespace"
	namespaceList.widget.visible = true
	namespaceList.widget.frame = true
	namespaceList.widget.template = tpl("namespace", `{{.metadata.name | printf "%-50.50s" }}`)

	resourceMenu = newNmenu("resourcesMenu", 1, 4, maxX-2, 16)
	resourceMenu.widget.visible = true
	resourceMenu.widget.title = fmt.Sprintf("[R]esources - %s", resourceCategories[selectedResourceCategoryIndex])
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

	resourceItemDetailsWidget = newTextWidget("text", "resource item details", false, 1, sepYAt, maxX-2, maxY-sepYAt-1)

	helpWidget = newTextWidget("help", "HELP", false, maxX/2-32, 5, 64, maxY-10)

	execWidget = newShellWidget("exec", "execWidget", 2, 5, maxX-4, maxY-10)

	errorWidget = newTextWidget("error", "ERROR", false, 10, 7, maxX-20, 10)
	errorWidget.wrap = true

	g.SetManager(clusterList.widget, clusterResourcesWidget, namespaceList.widget, resourceMenu.widget, resourcesItemDetailsMenu.widget, searchmodeWidget, resourceItemsList.widget, resourceItemDetailsWidget, helpWidget, errorWidget, execWidget)

}

func resourceItemDetailsViews() []interface{} {
	res := currentResource()
	ret := make([]interface{}, 0)
	for _, v := range cfg.detailViews(res) {
		ret = append(ret, v)
	}
	return ret
}

func filterResources(res []resourceType) []interface{} {
	ret := make([]interface{}, 0)
	for _, r := range res {
		ns := currentNamespace()
		resItems, err := backend.resourceItems(ns, r)
		if err != nil {
			showError(fmt.Sprintf("Can't load %s items of namespace %s", r, ns), err)
		}

		if len(resItems) > 0 {
			ret = append(ret, r)
		}
	}
	return ret
}

func showError(mess string, err error) {

	g.Update(func(gui *gocui.Gui) error {
		co := []interface{}{mess, err}
		errorWidget.setContent(co, tpl("error", errorTemplate))
		setState(errorState)
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
	resourceMenu.widget.title = fmt.Sprintf("[R]esources - %s", resourceCategories[selectedResourceCategoryIndex])

	resourceMenu.widget.selectedItem = 0
	resourceMenu.widget.items = resources()

	selRes := currentResource()
	ns := currentNamespace()
	resItems, err := backend.resourceItems(ns, selRes)
	if err != nil {
		showError(fmt.Sprintf("Can't load %s items of namespace %s", selRes, ns), err)
		return
	}
	resourceItemsList.widget.items = resItems
	resourceItemsList.widget.title = fmt.Sprintf("%s - Items", selRes.Name)
	resourceItemsList.widget.template = resourceListTpl(selRes)
	if resourceItemsList.widget.selectedItem >= len(resourceItemsList.widget.items) {
		resourceItemsList.widget.selectedItem = 0
	}
}

func newContext() {
	backend = newRestyBackend(cfg.contexts[clusterList.widget.selectedItem])
	backend.resetCache()
	res := cfg.resourcesOfName("namespaces")
	resItems, err := backend.resourceItems("", res)
	if err != nil {
		showError(fmt.Sprintf("Can't load %s items", res), err)
		return
	}
	namespaceList.widget.items = resItems
	namespaceList.widget.selectedItem = 0
	clusterRes := clusterRes()
	clusterResourcesWidget.setContent(&clusterRes, tpl("clusterResources", clusterResourcesTemplate))
	findResourceCategoryWithResources(1)
	updateKubectlContext()
}

func updateKubectlContext() {
	cmd := kubectl("config", "use-context", backend.context.Name)
	out, err := cmd.Output()
	if err != nil {
		showError(fmt.Sprint("Problem executing kubectl"), err)
		return
	}
	infolog.Printf("kubectl: %s", out)
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

func deleteResource() {
	ns := currentNamespace()
	res := currentResource()
	rname := currentResourceItemName()
	_, err := backend.delete(ns, res, rname)
	if err != nil {
		showError(fmt.Sprintf("Can't delete %s on namespace %s with name '%s' ", res.Name, ns, rname), err)
	}
}

func scaleResource(replicas int) {
	res := currentResource()
	ns := currentNamespace()
	rname := currentResourceItemName()
	_, err := backend.scale(ns, res, rname, replicas)
	if err != nil {
		showError(fmt.Sprintf("Can't scale %s on namespace %s with name '%s' ", res.Name, ns, rname), err)
	}
}

func newResource() {
	selRes := currentResource()
	selNs := currentNamespace()

	resItems, err := backend.resourceItems(selNs, selRes)
	if err != nil {
		showError(fmt.Sprintf("Can't load %s items of namespace ", selRes.Name, selNs), err)
		return
	}
	resourceItemsList.widget.items = resItems
	resourceItemsList.widget.title = fmt.Sprintf("%s - Items", selRes.Name)
	resourceItemsList.widget.template = resourceListTpl(selRes)
	resourceItemsList.widget.selectedPage = 0
	resourceItemsList.widget.selectedItem = 0
}

func setResourceItemDetailsPart() {

	ns := currentNamespace()
	res := currentResource()
	rname := currentResourceItemName()
	view := currentResourceItemDetailsView()

	resourceItemDetailsWidget.xOffset = 0
	resourceItemDetailsWidget.yOffset = 0

	details, err := backend.getDetail(ns, res, rname, view)
	if err != nil {
		showError(fmt.Sprintf("Can't load %s details of namespace %s from item '%s'  ", res, ns, rname), err)
		return
	}

	resourceItemDetailsWidget.setContent(details, resourceTpl(res, view))
	resourceItemDetailsWidget.title = fmt.Sprintf("%s - %s  details ", res.Name, rname)
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

func createPortforwardProxy(podName string, pm portMapping) error {
	ns := currentNamespace()
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

func removePortforwardProxyofPod(podName string) error {
	ns := currentNamespace()
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

func bindKeys() {
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'h', mod: gocui.ModNone}, showHelpCommand)
	bindKey(g, keyEventType{Viewname: helpWidget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, quitWidgetCommand)
	bindKey(g, keyEventType{Viewname: helpWidget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, scrollDownHelpCommand)
	bindKey(g, keyEventType{Viewname: helpWidget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, scrollUpHelpCommand)

	bindKey(g, keyEventType{Viewname: errorWidget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, quitWidgetCommand)

	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyCtrlC, mod: gocui.ModNone}, quitCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowRight, mod: gocui.ModNone}, nextResourceCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowLeft, mod: gocui.ModNone}, previousResourceCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, nextLineCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousLineCommand)

	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'c', mod: gocui.ModNone}, gotoSelectContextStateCommand)
	// bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'C', mod: gocui.ModNone}, previousContextCommand)
	// bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'f', mod: gocui.ModNone}, nextNamespaceCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'n', mod: gocui.ModNone}, gotoSelectNamespaceStateCommand)
	// bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'F', mod: gocui.ModNone}, previousNamespaceCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, toggleResourceItemDetailsCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'r', mod: gocui.ModNone}, nextResourceCategoryCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, nextResourceItemListPageCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, previousResourceItemListPageCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeySpace, mod: gocui.ModNone}, reloadCommand)

	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: gocui.KeyDelete, mod: gocui.ModNone}, deleteCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '+', mod: gocui.ModNone}, scaleUpCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '-', mod: gocui.ModNone}, scaleDownCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'm', mod: gocui.ModNone}, nameSortCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'a', mod: gocui.ModNone}, ageSortCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'x', mod: gocui.ModNone}, execShellCommand0)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '1', mod: gocui.ModNone}, execShellCommand0)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '2', mod: gocui.ModNone}, execShellCommand1)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '3', mod: gocui.ModNone}, execShellCommand2)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'X', mod: gocui.ModNone}, execBashCommand0)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '4', mod: gocui.ModNone}, execBashCommand0)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '5', mod: gocui.ModNone}, execBashCommand1)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: '6', mod: gocui.ModNone}, execBashCommand2)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'p', mod: gocui.ModNone}, portForwardSamePortCommand)
	bindKey(g, keyEventType{Viewname: resourceItemsList.widget.name, Key: 'P', mod: gocui.ModNone}, portForwardCommand)

	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowRight, mod: gocui.ModNone}, nextResourceItemDetailPartCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowLeft, mod: gocui.ModNone}, previousResourceItemDetailPartCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, scrollDownCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, scrollUpCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlA, mod: gocui.ModNone}, scrollLeftCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlD, mod: gocui.ModNone}, scrollRightCommand)

	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, pageDownCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, pageUpCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyHome, mod: gocui.ModNone}, homeCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyEnd, mod: gocui.ModNone}, endCommand)

	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, toggleResourceItemDetailsCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlN, mod: gocui.ModNone}, findNextCommand)
	bindKey(g, keyEventType{Viewname: searchmodeWidget.name, Key: gocui.KeyCtrlP, mod: gocui.ModNone}, findPreviousCommand)

	bindKey(g, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, selectNamespaceCommand)
	bindKey(g, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousNamespaceCommand)
	bindKey(g, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, nextNamespaceCommand)
	bindKey(g, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, nextNamespacePageCommand)
	bindKey(g, keyEventType{Viewname: namespaceList.widget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, previousNamespacePageCommand)

	bindKey(g, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyEnter, mod: gocui.ModNone}, selectContextCommand)
	bindKey(g, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousContextCommand)
	bindKey(g, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyArrowDown, mod: gocui.ModNone}, nextContextCommand)
	bindKey(g, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyPgdn, mod: gocui.ModNone}, nextContextPageCommand)
	bindKey(g, keyEventType{Viewname: clusterList.widget.name, Key: gocui.KeyPgup, mod: gocui.ModNone}, previousContextPageCommand)

}

func toIfc(cis []contextType) []interface{} {
	ifcs := make([]interface{}, len(cis))
	for i, ci := range cis {
		ifcs[i] = ci
	}
	return ifcs
}
