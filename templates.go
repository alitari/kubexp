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

	"github.com/alitari/gocui"
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

func tplNoFunc(tplName, tplStr string) *template.Template {
	if templateCache[tplName] == nil {
		tpl := template.Must(template.New(tplName).Parse(tplStr))
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
	"ind":                   index,
	"count":                 count,
	"keys":                  keys,
	"fk":                    filterArrayOnKeyValue,
	"keyStr":                keyString,
	"ctx":                   contextName,
	"decode64":              decode64,
	"podLog":                podLog,
	"events":                eventsFor,
	"podRes":                podRes,
	"nodeRes":               nodeRes,
	"clusterRes":            clusterRes,
	"status":                status,
	"podStatus":             podStatus,
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

func podLog() interface{} {
	//infolog.Printf("-->logs: %s", string(backend.podLogs))
	return string(backend.podLogs)
}

func podsForNode(nodeName string) interface{} {
	podType := cfg.resourcesOfName("pods")
	pods := backend.resourceItems("", podType)
	result := filterArrayOnTpl(pods, "{{ spec.nodeName }}", nodeName)
	tracelog.Printf("found %v pods ", len(result))
	return result
}

func eventsFor(resourceType, name string) interface{} {
	eventType := cfg.resourcesOfName("events")
	evs := backend.resourceItems(selectedResourceItemNamespace(), eventType)
	result := filterArrayOnTpl(evs, "{{ .involvedObject.kind }},{{ .involvedObject.name }}", fmt.Sprintf("%s,%s", resourceType, name))
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
	capaCPU := val1(node, "{{ .status.capacity.cpu }}")
	capaMem := val1(node, "{{ .status.capacity.memory }}")
	capaPod := val1(node, "{{ .status.capacity.pods }}")
	alloCPU := val1(node, "{{ .status.allocatable.cpu }}")
	alloMem := val1(node, "{{ .status.allocatable.memory }}")
	alloPod := val1(node, "{{ .status.allocatable.pods }}")
	return NodeResourcesDef{ResDef: ResourcesDef{CPU: MustParse(capaCPU), Memory: MustParse(capaMem)}, Pods: toInt(capaPod)}, NodeResourcesDef{ResDef: ResourcesDef{CPU: MustParse(alloCPU), Memory: MustParse(alloMem)}, Pods: toInt(alloPod)}
}

func usageOfNode(node interface{}) (ResourcesDef, ResourcesDef) {
	pods := podsForNode(resItemName(node)).([]interface{})
	return usageOfPods(pods)
}

func usageOfCluster() (ResourcesDef, ResourcesDef) {
	podType := cfg.resourcesOfName("pods")
	pods := backend.resourceItems("*ALL*", podType)
	return usageOfPods(pods)
}

func resourcesOfCluster() (NodeResourcesDef, NodeResourcesDef) {
	nodeType := cfg.resourcesOfName("nodes")
	nodes := backend.resourceItems("", nodeType)
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

func resourcesOfPod(pod interface{}) (ResourcesDef, ResourcesDef) {
	tpl := `
	{{- range .spec.containers -}}
	;{{.resources.requests.cpu}},{{.resources.requests.memory}},{{.resources.limits.cpu}},{{.resources.limits.memory}}
	{{- end -}}
	`
	podresStr := val1(pod, tpl)
	podRess := strings.Split(podresStr[1:], ";")
	rpReq := ResourcesDef{CPU: MustParse("0m"), Memory: MustParse("0Mi")}
	rpLimit := ResourcesDef{CPU: MustParse("0m"), Memory: MustParse("0Mi")}
	for _, res := range podRess {
		ress := strings.Split(res, ",")
		if ress[0] == "<no value>" {
			ress[0] = "0m"
		}
		if ress[1] == "<no value>" {
			ress[1] = "0Mi"
		}
		if ress[2] == "<no value>" {
			ress[2] = "0m"
		}
		if ress[3] == "<no value>" {
			ress[3] = "0Mi"
		}
		req := ResourcesDef{CPU: MustParse(ress[0]), Memory: MustParse(ress[1])}
		lim := ResourcesDef{CPU: MustParse(ress[2]), Memory: MustParse(ress[3])}
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
		fallthrough
	case "Not ready":
		return colorYellowEmp(text)
	case "Running":
		fallthrough
	case "Succeeded":
		return colorGreenEmp(text)
	case "Failed":
		fallthrough
	case "Unknown":
		return colorRedEmp(text)
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

func labelValue(it interface{}, label string) string {
	return val1(it, fmt.Sprintf("{{ .metadata.labels.%s }}", label))
}

func annotations(it interface{}) string {
	return stripMap(val1(it, "{{ .metadata.annotations }}"))
}

func stripMap(s string) string {
	s = strings.Replace(s, "map[", "", -1)
	s = strings.Replace(s, "]", "", -1)
	s = strings.Replace(s, "[", "", -1)
	return s
}

func ports(it interface{}) []containerPort {
	res := make([]containerPort, 0)
	portNamesTpl := `
	{{- range .spec.containers -}}
	{{- range .ports -}}
	;{{- .name -}},{{- .containerPort -}}
	{{- end -}}
	{{- end -}}
	`
	portsStr := val1(it, portNamesTpl)[1:]
	ports := strings.Split(portsStr, ";")
	for _, ps := range ports {
		pss := strings.Split(ps, ",")
		portInt, _ := strconv.ParseInt(pss[1], 10, 32)
		res = append(res, containerPort{name: pss[0], port: int(portInt)})
	}
	return res

}

func portForwardPorts(pod interface{}, printFunc func(portMapping) string) string {
	name := resItemName(pod)
	ns := resItemNamespace(pod)
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
	return stripMap(val1(it, "{{ . }}"))
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

var stsTmpl = `
{{ range $ind, $cst := .containerStatuses }}
{{- .ready  }},
{{ end }}
`

func podStatus(status interface{}) string {
	phase := val1(status, "{{ .phase }}")
	if phase == "Running" {
		sts := val1(status, stsTmpl)
		if strings.Contains(sts, "false") {
			return "Not ready"
		}
		return phase
	}
	return phase
}

func fromChildrenWhenEquals(it interface{}, equalsKey, equalsValue, returnValueKey string) string {
	fil := filterArrayOnTpl(it, fmt.Sprintf("{{ .%s }}", equalsKey), equalsValue)
	if len(fil) <= 0 {
		return ""
	}
	val := val1(fil, fmt.Sprintf("{{ (index . 0).%s }}", returnValueKey))
	return val
}

func keyString(keyEvent interface{}) string {
	keyStr := ""

	k := keyEvent.(keyEventType).Key
	m := keyEvent.(keyEventType).mod

	switch k.(type) {
	case gocui.Key:
		switch k {
		case gocui.KeyCtrlC:
			keyStr = "Ctrl-C"
		case gocui.KeyTab:
			keyStr = "Tab"
		case gocui.KeyArrowDown:
			keyStr = "↓"
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
		case gocui.KeyCtrlA:
			keyStr = "Ctrl-a"
		case gocui.KeyCtrlD:
			keyStr = "Ctrl-d"
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
		case gocui.KeyCtrlO:
			keyStr = "Ctrl-o"
		case gocui.KeyDelete:
			if m == gocui.ModAlt {
				keyStr = "Alt-Delete"
			} else {
				keyStr = "Delete"
			}
		default:
			keyStr = fmt.Sprintf("%c", k)
		}

	case rune:
		keyStr = string(k.(rune))
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
	tpl := fmt.Sprintf("{{- range . -}},{{- .%s -}}{{- end -}}", key)
	val := val1(it, tpl)
	if len(val) > 0 {
		return val[1:]
	}
	return val
}

func resItemName(ri interface{}) string {
	return val1(ri, "{{ .metadata.name }}")
}
func resItemNamespace(ri interface{}) string {
	return val1(ri, "{{ .metadata.namespace }}")
}

func resItemContainers(ri interface{}) []string {
	contStr := val1(ri, "{{ range .spec.containers -}}{{ .name}},{{ end -}}")
	return strings.Split(contStr[:len(contStr)-1], ",")
}

func resItemCreationTimestamp(ri interface{}) string {
	return val1(ri, "{{ .metadata.creationTimestamp }}")
}

func val1(node interface{}, path string) string {
	tpl := tplNoFunc(path, path)
	buf := new(bytes.Buffer)
	err := tpl.Execute(buf, node)
	if err == nil {
		return buf.String()
	}
	return err.Error()
}

func filterArrayOnTpl(it interface{}, tmpl, equals string) []interface{} {
	r := make([]interface{}, 0)
	switch it.(type) {
	case []interface{}:
		itt := it.([]interface{})
		for _, v := range itt {
			if val1(v, tmpl) == equals {
				r = append(r, v)
			}
		}
	}
	return r
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

func printMap(it interface{}) string {
	return stripMap(fmt.Sprintf("%s", it))
}
