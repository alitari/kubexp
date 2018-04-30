package kubexp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/jroimartin/gocui"
	yaml "gopkg.in/yaml.v2"
)

var defaultTemplate = `{{.metadata.name | printf "%-50.50s"  }}`

var templateCache = map[string]*template.Template{}

func resourceListTpl(res resourceType) *template.Template {
	v := cfg.listView(res)
	return resourceTpl(res, v)
}

func resourceTpl(res resourceType, v viewType) *template.Template {
	return tpl(res.Name+v.Name, v.Template)
}

func tpl(tplName, tplStr string) *template.Template {
	if templateCache[tplName] == nil {
		tpl := template.Must(template.New(tplName).Funcs(templateFuncMap).Parse(tplStr))
		templateCache[tplName] = tpl
	}
	return templateCache[tplName]
}

var templateFuncMap = template.FuncMap{
	"printMap":              printMap,
	"header":                header,
	"fcwe":                  fromChildrenWhenEquals,
	"toJSON":                marshalJSON,
	"toYaml":                marshalYaml,
	"age":                   age,
	"fc":                    fromChildren,
	"printArray":            printArray,
	"mergeArrays":           mergeArrays,
	"ind":                   index,
	"count":                 count,
	"keys":                  keys,
	"fk":                    filterArrayOnKeyValue,
	"keyStr":                keyString,
	"ctx":                   contextName,
	"decode64":              decode64,
	"podLog":                podLog,
	"podExec":               podExec,
	"events":                eventsFor,
	"podRes":                podRes,
	"nodeRes":               nodeRes,
	"clusterRes":            clusterRes,
	"status":                status,
	"portForwardPortsShort": portForwardPortsShort,
	"portForwardPortsLong":  portForwardPortsLong,

	"green":    colorGreen,
	"greenEmp": colorGreenInverse,
	"greenInv": colorGreenEmp,

	"grey": colorGrey,

	"red":    colorRed,
	"redEmp": colorRedInverse,
	"redInv": colorRedEmp,

	"yellow":    colorYellow,
	"yellowEmp": colorYellowInverse,
	"yellowInv": colorYellowEmp,

	"blue":    colorBlue,
	"blueEmp": colorBlueInverse,
	"blueInv": colorBlueEmp,

	"cyan":    colorCyan,
	"cyanEmp": colorCyanInverse,
	"cyanInv": colorCyanEmp,

	"whiteEmp": colorWhiteEmp,
	"whiteInv": colorWhiteInverse,

	"contextColor":    colorContext,
	"contextColorEmp": colorContextEmp,
	"colorPhase":      colorPhase,
	//"colorDesiredCount" : colorDesiredCount,
}

func header(header string, rootVal, val interface{}) interface{} {
	switch rootVal.(type) {
	case map[string]interface{}:
		if rootVal.(map[string]interface{})["header"] == "true" {
			return header
		}
	}
	if val == nil {
		return ""
	}
	switch val.(type) {
	case float64:
		return strconv.Itoa(int(val.(float64)))
	}
	return val
}

func decode64(it interface{}) string {
	switch it.(type) {
	case string:
		data, err := base64.StdEncoding.DecodeString(it.(string))
		if err != nil {
			return fmt.Sprintf("error:", err)
		}
		return string(data)
	}
	return ""
}

func podLog(podName, containerName string) interface{} {
	logs, _ := backend.readPodLogs(currentNamespace(), podName, containerName)
	return logs
}

func podExec(podName, containerName, command string) interface{} {
	resp, err := backend.execPodCommand(currentNamespace(), podName, containerName, command)
	if err != nil {
		return fmt.Sprintf("error=%v", err)
	}
	return resp
}

func podsForNode(nodeName string) interface{} {
	pods := backend.resourceItems("", cfg.resourcesOfName("pods"))

	result := filterArray(pods, func(item interface{}) bool {
		podnn := val(item, []interface{}{"spec", "nodeName"}, "").(string)
		return nodeName == podnn
	})

	tracelog.Printf("found %v pods ", len(result.([]interface{})))
	return result
}

func eventsFor(resourceType, name string) interface{} {
	evs := backend.resourceItems(currentNamespace(), cfg.resourcesOfName("events"))

	result := filterArray(evs, func(item interface{}) bool {
		ino := item.(map[string]interface{})["involvedObject"]
		ki := ino.(map[string]interface{})["kind"].(string)
		na := ino.(map[string]interface{})["name"].(string)
		return ki == resourceType && na == name
	})
	return result
}

// ResourcesDef ...
type ResourcesDef struct {
	CPU    Quantity
	Memory Quantity
}

// NodeResourcesDef ...
type NodeResourcesDef struct {
	ResDef ResourcesDef
	Pods   int
}

// ClusterResourcesDef ...
type ClusterResourcesDef struct {
	Capacity   *NodeResourcesDef
	Alloctable *NodeResourcesDef
	Requested  *ResourcesDef
	Limit      *ResourcesDef
}

// PercentCPU ...
func (c *ClusterResourcesDef) PercentCPU() string {
	return percent(c.Alloctable.ResDef.CPU.Value(), c.Requested.CPU.Value())
}

// PercentMem ...
func (c *ClusterResourcesDef) PercentMem() string {
	return percent(c.Alloctable.ResDef.Memory.Value(), c.Requested.Memory.Value())
}

func percent(t, p int64) string {
	perc := (float64(p) / float64(t)) * 100
	return fmt.Sprintf("%.0f%%", perc)
}

func (r *ResourcesDef) add(mr ResourcesDef) {
	r.CPU.Add(mr.CPU)
	r.Memory.Add(mr.Memory)
}

func (r *ResourcesDef) String() string {
	return fmt.Sprintf("cpu:%s/memory:%s", r.CPU.String(), r.Memory.String())
}

func (r *NodeResourcesDef) add(mr NodeResourcesDef) {
	r.ResDef.add(mr.ResDef)
	r.Pods = r.Pods + mr.Pods
}

func (r *NodeResourcesDef) String() string {
	return r.ResDef.String() + fmt.Sprintf("/pods:%v", r.Pods)
}

func clusterRes() interface{} {
	cap, allo := resourcesOfCluster()
	req, lim := usageOfCluster()
	return &ClusterResourcesDef{Capacity: &cap, Alloctable: &allo, Requested: &req, Limit: &lim}
}

func podRes(pod interface{}) interface{} {
	req, lim := resourcesOfPod(pod)
	return fmt.Sprintf("requests: %s,  limit: %s", req.String(), lim.String())
}

func nodeRes(node interface{}) interface{} {
	cap, allo := resourcesOfNode(node)
	req, limit := usageOfNode(node)
	reqCPUPerc := percent(allo.ResDef.CPU.Value(), req.CPU.Value())
	reqMemPerc := percent(allo.ResDef.Memory.Value(), req.Memory.Value())
	limCPUPerc := percent(allo.ResDef.CPU.Value(), limit.CPU.Value())
	limMemPerc := percent(allo.ResDef.Memory.Value(), limit.Memory.Value())
	return fmt.Sprintf("capacity: %s,  allocatable: %s, requested: %s ( %s/%s) limits: %s ( %s/%s)", cap.String(), allo.String(), req.String(), reqCPUPerc, reqMemPerc, limit.String(), limCPUPerc, limMemPerc)
}

func resourcesOfNode(node interface{}) (NodeResourcesDef, NodeResourcesDef) {
	capaCPU := val(node, []interface{}{"status", "capacity", "cpu"}, "0m").(string)
	capaMem := val(node, []interface{}{"status", "capacity", "memory"}, "0Ki").(string)
	capaPod := val(node, []interface{}{"status", "capacity", "pods"}, "0").(string)
	alloCPU := val(node, []interface{}{"status", "allocatable", "cpu"}, "0m").(string)
	alloMem := val(node, []interface{}{"status", "allocatable", "memory"}, "0Ki").(string)
	alloPod := val(node, []interface{}{"status", "allocatable", "pods"}, "0").(string)
	return NodeResourcesDef{ResDef: ResourcesDef{CPU: MustParse(capaCPU), Memory: MustParse(capaMem)}, Pods: toInt(capaPod)}, NodeResourcesDef{ResDef: ResourcesDef{CPU: MustParse(alloCPU), Memory: MustParse(alloMem)}, Pods: toInt(alloPod)}
}

func usageOfNode(node interface{}) (ResourcesDef, ResourcesDef) {
	pods := podsForNode(resourceName(node)).([]interface{})
	return usageOfPods(pods)
}

func usageOfCluster() (ResourcesDef, ResourcesDef) {
	//TODO: ERROR currentNameSpace
	pods := backend.resourceItems(currentNamespace(), cfg.resourcesOfName("pods"))
	return usageOfPods(pods)
}

func resourcesOfCluster() (NodeResourcesDef, NodeResourcesDef) {
	nodes := backend.resourceItems("", cfg.resourcesOfName("nodes"))
	rCap, rAllo := resourcesOfNode(nodes[0])
	for _, n := range nodes[1:] {
		cap, allo := resourcesOfNode(n)
		rCap.add(cap)
		rAllo.add(allo)
	}
	return rCap, rAllo
}

func usageOfPods(pods []interface{}) (ResourcesDef, ResourcesDef) {
	if len(pods) > 0 {
		rpReq, rpLimit := resourcesOfPod(pods[0])
		for _, p := range pods[1:] {
			req, lim := resourcesOfPod(p)
			rpReq.add(req)
			rpLimit.add(lim)
		}
		return rpReq, rpLimit
	}
	return ResourcesDef{}, ResourcesDef{}
}

func resourcesOfContainer(container interface{}) (ResourcesDef, ResourcesDef) {
	reqCPU := val(container, []interface{}{"resources", "requests", "cpu"}, "0m").(string)
	reqMem := val(container, []interface{}{"resources", "requests", "memory"}, "0Ki").(string)
	limCPU := val(container, []interface{}{"resources", "limits", "cpu"}, "0m").(string)
	limMem := val(container, []interface{}{"resources", "limits", "memory"}, "0Ki").(string)
	return ResourcesDef{CPU: MustParse(reqCPU), Memory: MustParse(reqMem)}, ResourcesDef{CPU: MustParse(limCPU), Memory: MustParse(limMem)}
}

func resourcesOfPod(pod interface{}) (ResourcesDef, ResourcesDef) {
	spec := pod.(map[string]interface{})["spec"]
	cons := spec.(map[string]interface{})["containers"].([]interface{})
	rpReq, rpLimit := resourcesOfContainer(cons[0])
	for _, c := range cons[1:] {
		req, lim := resourcesOfContainer(c)
		rpReq.add(req)
		rpLimit.add(lim)
	}
	return rpReq, rpLimit
}

func toInt(s string) int {
	res, err := strconv.Atoi(s)
	if err != nil {
		errorlog.Panicf("error: %v", err)
	}
	return res
}

func colorPhase(text string) string {
	trimmed := strings.TrimSpace(text)
	switch trimmed {
	case "Pending":
		return colorYellow(text)
	case "Running":
		fallthrough
	case "Succeeded":
		return colorGreen(text)
	case "Failed":
		fallthrough
	case "Unknown":
		return colorRed(text)
	}
	return text
}

func colorGrey(text string) string {
	return colorizeText(text, 0, len(text), greyInlineColor)
}

func colorRed(text string) string {
	return colorizeText(text, 0, len(text), redEmpInlineColor)
}
func colorRedEmp(text string) string {
	return colorizeText(text, 0, len(text), redEmpInlineColor)
}
func colorRedInverse(text string) string {
	return colorizeText(text, 0, len(text), redInverseInlineColor)
}
func colorGreenEmp(text string) string {
	return colorizeText(text, 0, len(text), greenEmpInlineColor)
}
func colorGreen(text string) string {
	return colorizeText(text, 0, len(text), greenInlineColor)
}
func colorGreenInverse(text string) string {
	return colorizeText(text, 0, len(text), greenInverseInlineColor)
}
func colorYellowEmp(text string) string {
	return colorizeText(text, 0, len(text), yellowEmpInlineColor)
}
func colorYellow(text string) string {
	return colorizeText(text, 0, len(text), yellowInlineColor)
}
func colorYellowInverse(text string) string {
	return colorizeText(text, 0, len(text), yellowInverseInlineColor)
}
func colorBlueEmp(text string) string {
	return colorizeText(text, 0, len(text), blueEmpInlineColor)
}
func colorBlue(text string) string {
	return colorizeText(text, 0, len(text), blueInlineColor)
}
func colorBlueInverse(text string) string {
	return colorizeText(text, 0, len(text), blueInverseInlineColor)
}

func colorMagentaEmp(text string) string {
	return colorizeText(text, 0, len(text), magentaEmpInlineColor)
}
func colorMagenta(text string) string {
	return colorizeText(text, 0, len(text), magentaInlineColor)
}

func colorMagentaInverse(text string) string {
	return colorizeText(text, 0, len(text), magentaInverseInlineColor)
}

func colorCyanEmp(text string) string {
	return colorizeText(text, 0, len(text), cyanEmpInlineColor)
}

func colorCyan(text string) string {
	return colorizeText(text, 0, len(text), cyanInlineColor)
}

func colorCyanInverse(text string) string {
	return colorizeText(text, 0, len(text), cyanInverseInlineColor)
}

func colorWhiteInverse(text string) string {
	return colorizeText(text, 0, len(text), whiteInverseInlineColor)
}

func colorWhiteEmp(text string) string {
	return colorizeText(text, 0, len(text), whiteEmpInlineColor)
}

func colorContext(text string) string {
	context := cfg.contexts[clusterList.widget.selectedItem]
	return colorizeText(text, 0, len(text), strToInlineColor(context.color, false))
}

func colorContextEmp(text string) string {
	context := cfg.contexts[clusterList.widget.selectedItem]
	return colorizeText(text, 0, len(text), strToInlineColor(context.color, true))
}

func index(it interface{}, i int) interface{} {
	switch it.(type) {
	case []interface{}:
		a := it.([]interface{})
		if i < len(a) {
			return a[i]
		}
	}
	return map[string]interface{}{}
}

func count(it interface{}) interface{} {
	switch it.(type) {
	case []interface{}:
		return strconv.Itoa(len(it.([]interface{})))
	}
	return ""
}

func marshalJSON(data interface{}) string {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func marshalYaml(data interface{}) string {
	b, err := yaml.Marshal(data)
	if err != nil {
		return ""
	}
	return string(b)
}

func resourceName(it interface{}) string {
	return val(it, []interface{}{"metadata", "name"}, "not found").(string)
}

func labels(it interface{}) map[string]interface{} {
	return valMap(it, []interface{}{"metadata", "labels"})
}

func ports(it interface{}) []containerPort {
	res := make([]containerPort, 0)
	cons := valArray(it, []interface{}{"spec", "containers"})
	for _, c := range cons {
		ports := valArray(c, []interface{}{"ports"})
		for _, p := range ports {
			cp := val(p, []interface{}{"containerPort"}, "")
			cpName := val(p, []interface{}{"name"}, "").(string)
			res = append(res, containerPort{name: cpName, port: int(cp.(float64))})
		}
	}
	return res

}

func portForwardPorts(pod interface{}, printFunc func(portMapping) string) string {
	name := resourceName(pod)
	ns := currentNamespace()
	var res bytes.Buffer
	pfl := portforwardProxies[ns+"/"+name]
	if pfl != nil {
		for _, pf := range pfl {
			res.WriteString(printFunc(pf.mapping) + ",")
		}
		res.Truncate(res.Len() - 1)
		return res.String()
	}
	return ""
}

func portForwardPortsShort(pod interface{}) string {
	return portForwardPorts(pod, func(pm portMapping) string {
		return strconv.Itoa((pm.destPort))
	})
}

func portForwardPortsLong(pod interface{}) string {
	return portForwardPorts(pod, func(pm portMapping) string {
		s := fmt.Sprintf("%s[%v -> %v]", pm.containerPort.name, pm.destPort, pm.containerPort.port)
		return s
	})
}

func annotations(it interface{}) map[string]interface{} {
	return valMap(it, []interface{}{"metadata", "annotations"})
}

func age(it interface{}) string {
	switch it.(type) {
	case string:
		return time2age(totime(it.(string)))
	}
	return ""
}

func filterArrayOnKeyValue(it interface{}, key interface{}, value interface{}) interface{} {
	return filterArray(it, func(item interface{}) bool {
		switch item.(type) {
		case map[string]interface{}:
			a := item.(map[string]interface{})
			return a[key.(string)] == value
		case map[interface{}]interface{}:
			a := item.(map[interface{}]interface{})
			return a[key] == value
		}
		return false
	})
}

func getFloat(it interface{}, key string) float64 {
	switch it.(type) {
	case map[string]interface{}:
		fl := it.(map[string]interface{})[key]
		switch fl.(type) {
		case float64:
			return fl.(float64)
		}
	}
	return float64(0)
}

func keys(it interface{}) interface{} {
	r := make([]string, 0)
	switch it.(type) {
	case map[string]interface{}:
		itt := it.(map[string]interface{})
		for k := range itt {
			r = append(r, k)
		}
	}
	sort.Strings(r)
	return r
}

func printArray(it interface{}) string {
	s := printArray0(it, func(itt interface{}) string {
		vs := fmt.Sprintf("%v", itt)
		return strings.Trim(vs, "\"")
	}, ",", "", "")
	return s.(string)
}

func status(desired, ready interface{}) string {
	switch desired.(type) {
	case float64:
		desInt := int(desired.(float64))
		readyInt := 0
		switch ready.(type) {
		case float64:
			readyInt = int(ready.(float64))
		}
		if desInt != readyInt {
			return "Pending"
		}
	}
	return "Succeeded"
}

func fromChildrenWhenEquals(it interface{}, equalsKey, equalsValue, returnValueKey, notFoundValue string) interface{} {
	fil := filterArray(it, func(itt interface{}) bool {
		switch itt.(type) {
		case map[string]interface{}:
			mcc := itt.(map[string]interface{})
			return mcc[equalsKey] == equalsValue
		}
		return false
	})
	return val(fil, []interface{}{0, returnValueKey}, notFoundValue)
}

func mergeArrays(format string, it0 interface{}, key0 string, it1 interface{}, key1 string) interface{} {
	a0 := toStringArray(it0, key0)
	a1 := toStringArray(it1, key1)
	s := make([]string, 0)
	for i := 0; i < min(len(a0), len(a1)); i++ {
		s0 := strings.Trim(a0[i], " []\"")
		s1 := strings.Trim(a1[i], " []\"")
		s = append(s, fmt.Sprintf(format, s0, s1))
	}
	return strings.Join(s, ",")

}

func keyString(key interface{}) string {
	keyStr := ""
	switch key.(type) {
	case gocui.Key:
		k := key.(gocui.Key)
		switch k {
		case gocui.KeyCtrlC:
			keyStr = "Ctrl-C"
		case gocui.KeyTab:
			keyStr = "Tab"
		case gocui.KeyArrowDown:
			keyStr = "ArrowDown"
		case gocui.KeyArrowRight:
			keyStr = "ArrowRight"
		case gocui.KeyArrowLeft:
			keyStr = "ArrowLeft"
		case gocui.KeyArrowUp:
			keyStr = "ArrowUp"
		case gocui.KeyEnter:
			keyStr = "Enter"
		case gocui.KeyHome:
			keyStr = "Home"
		case gocui.KeyEnd:
			keyStr = "End"
		case gocui.KeyCtrlN:
			keyStr = "Ctrl-n"
		case gocui.KeyCtrlP:
			keyStr = "Ctrl-p"
		case gocui.KeyCtrl2:
			keyStr = "Ctrl-2"
		case gocui.KeyCtrl3:
			keyStr = "Ctrl-3"
		case gocui.KeySpace:
			keyStr = "Space"
		case gocui.KeyPgdn:
			keyStr = "Page Down"
		case gocui.KeyPgup:
			keyStr = "Page Up"
		case gocui.KeyDelete:
			keyStr = "Delete"
		default:
			keyStr = fmt.Sprintf("%v", key)
		}
	case rune:
		keyStr = string(key.(rune))
	default:
		keyStr = "error"
	}
	return keyStr
}

func contextName(viewName interface{}) string {
	switch viewName.(type) {
	case string:
		switch viewName {
		case "search":
			return "Detail"
		case "resourceItems":
			return "Browse"
		case "":
			return "All"
		}
	}
	return viewName.(string)
}

func fromChildren(it interface{}, key string) string {
	return strings.Join(toStringArray(it, key), ",")
}

func toStringArray(it interface{}, key string) []string {
	return mapToStringArray(it, func(itt interface{}) string {
		switch itt.(type) {
		case map[string]interface{}:
			return fmt.Sprintf("%v", itt.(map[string]interface{})[key])
		}
		return ""
	})
}

//-- base functions

func valMap(node interface{}, path []interface{}) map[string]interface{} {
	res := val(node, path, "not found")
	switch res.(type) {
	case map[string]interface{}:
		return res.(map[string]interface{})
	default:
		return map[string]interface{}{}
	}
}

func valArray(node interface{}, path []interface{}) []interface{} {
	res := val(node, path, "not found")
	switch res.(type) {
	case []interface{}:
		return res.([]interface{})
	default:
		return []interface{}{}
	}
}

func resItemName(ri interface{}) string {
	return val(ri, []interface{}{"metadata", "name"}, "").(string)
}

func val(node interface{}, path []interface{}, notFoundVal string) interface{} {
	for _, p := range path {
		switch p.(type) {
		case string:
			switch node.(type) {
			case map[string]interface{}:
				node = node.(map[string]interface{})[p.(string)]
			case map[interface{}]interface{}:
				node = node.(map[interface{}]interface{})[p.(string)]
			default:
				return notFoundVal
			}
		case int:
			switch node.(type) {
			case []interface{}:
				nodet := node.([]interface{})
				if len(nodet) > p.(int) {
					node = nodet[p.(int)]
				} else {
					return notFoundVal
				}
			default:
				return notFoundVal
			}
		}
	}
	if node == nil {
		return notFoundVal
	}
	return node
}

func filterArray(it interface{}, f func(item interface{}) bool) interface{} {
	r := make([]interface{}, 0)
	switch it.(type) {
	case []interface{}:
		itt := it.([]interface{})
		for _, v := range itt {
			if f(v) {
				r = append(r, v)
			}
		}
	}
	return r
}

func mapArray(it interface{}, f func(item interface{}) interface{}) interface{} {
	r := make([]interface{}, 0)
	switch it.(type) {
	case []interface{}:
		itt := it.([]interface{})
		for _, v := range itt {
			r = append(r, f(v))
		}
	}
	return r
}

func filterMap(it interface{}, f func(key string, value interface{}) bool) map[string]interface{} {
	r := map[string]interface{}{}
	switch it.(type) {
	case map[string]interface{}:
		itt := it.(map[string]interface{})
		for k, v := range itt {
			if f(k, v) {
				r[k] = v
			}
		}
	}
	return r
}

func mapToStringArray(it interface{}, f func(item interface{}) string) []string {
	r := make([]string, 0)
	switch it.(type) {
	case []interface{}:
		itt := it.([]interface{})
		for _, v := range itt {
			r = append(r, f(v))
		}
	}
	return r
}

func printArray0(it interface{}, f func(item interface{}) string, separator, beginLimiter, endLimiter string) interface{} {
	sa := mapToStringArray(it, f)
	sort.Strings(sa)
	var buffer bytes.Buffer
	buffer.WriteString(beginLimiter)
	for i, v := range sa {
		s := f(v)
		buffer.WriteString(s)
		if i < len(sa)-1 {
			buffer.WriteString(separator)
		}
	}
	buffer.WriteString(endLimiter)
	return buffer.String()
}

func printMap(it interface{}) string {
	var buffer bytes.Buffer
	switch it.(type) {
	case map[string]interface{}:
		itt := it.(map[string]interface{})
		keys := make([]string, 0, len(itt))
		for key := range itt {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buffer.WriteString(fmt.Sprintf("%s:%s,", k, itt[k]))
		}
	}
	return strings.TrimRight(buffer.String(), ",")
}
