package kubexp

import (
	"os"
	// "fmt"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

func setup(level string) {
	logLevel = &level
	initLog(os.Stdout)
}

func shutdown() {
}

func TestMain(m *testing.M) {
	setup("trace")
	code := m.Run()
	shutdown()
	os.Exit(code)
}

func Test_funcMap_pods(t *testing.T) {
	data := loadData("pods.json")
	checkTpl(t, "{{(index .items 0).metadata.name  }}", data, "calico-etcd-bkcr2")
	checkTpl(t, `{{ fcwe (index .items 0).status.conditions "status" "True" "type" }}`, data, "Initialized")
}

func Test_val(t *testing.T) {
	data := loadData("pods.json")
	firstPodUID := val1(data, "{{ ( index .items 0 ).metadata.uid }}")
	secondPodUID := val1(data, "{{ ( index .items 1 ).metadata.uid }}")
	check(t, firstPodUID, "664ce375-5059-11e7-90fb-e8039a27cebe")
	check(t, secondPodUID, "67d9fe5c-5059-11e7-90fb-e8039a27cebe")
}

func Test_labels(t *testing.T) {
	data := loadData("pods-minio-minio-3709128164-vdwk1.json")
	applabel := labelValue(data, "app")
	check(t, applabel, "minio-minio")
}

func Test_ports(t *testing.T) {
	data := loadData("pods-minio-minio-3709128164-vdwk1.json")
	ports := ports(data)
	checkInt(t, ports[0].port, 9000)
	check(t, ports[0].name, "service0")
	checkInt(t, ports[1].port, 9001)
	check(t, ports[1].name, "service1")
}

func Test_annotations(t *testing.T) {
	data := loadData("pods-minio-minio-3709128164-vdwk1.json")
	anno := annotations(data)
	check(t, anno, "anno1:anno1-value")
}

func Test_funcMap_serviceAccounts(t *testing.T) {
	data := loadData("serviceaccounts.json")
	checkTpl(t, `{{ fc (index .items 0).secrets "name"   }}`, data, "default-token-c93pb,my-secret")
}

func Test_funcMap_persitentVolumes(t *testing.T) {
	data := loadData("persistentvolumes.json")
	checkTpl(t, `{{ printf "%s/%s" (index .items 0).spec.claimRef.namespace (index .items 0).spec.claimRef.name  }}`, data, "default/myclaim-1")
}

func Test_funcMap_endpoints(t *testing.T) {
	data := loadData("endpoints.json")
	checkTpl(t, `{{ (index (index (index .items 0).subsets 0).addresses 0).ip }}`, data, "192.168.0.87")
	checkTpl(t, `{{ (index (index (index .items 0).subsets 0).ports 0).port }}`, data, "6443")
}

func Test_nodeResources(t *testing.T) {
	data := loadData("nodes-rv515.localdomain.json")
	capa, allo := resourcesOfNode(data)
	t.Logf("Node resource capacity: %s", capa.String())
	check(t, capa.String(), "cpu:2/memory:7991092Ki/pods:110")
	check(t, allo.String(), "cpu:2/memory:7888692Ki/pods:110")
}

func Test_podResources(t *testing.T) {
	pod := loadData("multicontainer.json")
	req, limit := resourcesOfPod(pod)
	check(t, req.String(), "cpu:360m/memory:356Mi")
	check(t, limit.String(), "cpu:1990m/memory:3Gi")
}

func Test_printMap(t *testing.T) {
	m := map[string]interface{}{"alex": "Hello", "bread": "egg"}
	checkContains(t, printMap(m), "bread")
	checkContains(t, printMap(m), "alex")
}

// func Test_helpTemplate(t *testing.T) {
//
// 	g, _ := gocui.NewGui(gocui.OutputNormal)
// 	bindKey(g, keyEventType{Viewname: "", Key: gocui.KeyCtrlC, mod: gocui.ModNone}, quitCommand)
// 	bindKey(g, keyEventType{Viewname: "resourceItems", Key: gocui.KeyArrowUp, mod: gocui.ModNone}, previousLineCommand)
// 	data := keyBindings
// 	exp := `
//  _  __    _         ___          _
// | |/ /  _| |__  ___| __|_ ___ __| |___ _ _ ___ _ _
// | ' < || | '_ \/ -_) _|\ \ / '_ \ / _ \ '_/ -_) '_|
// |_|\_\_,_|_.__/\___|___/_\_\ .__/_\___/_| \___|_|
//                            |_|           v0.1
// Build: 1ed67788db374e13c57f0703315d6c2a71c97a7b
// Context         Key              Command
// All             Ctrl-C           Quit
// Browse          ArrowUp          Previous resource item         `
// 	checkTpl(t, helpTemplate, data, exp)
// }

func checkContains(t *testing.T, a string, expected string) {
	if !strings.Contains(a, expected) {
		t.Errorf("unexpected result: expected:'%s', is in '%s' ", expected, a)
	}
}

func check(t *testing.T, a string, expected string) {
	if a != expected {
		t.Errorf("unexpected result: expected:'%s', but is '%s' ", expected, a)
	}
}

func checkInt(t *testing.T, a int, expected int) {
	if a != expected {
		t.Errorf("unexpected result: expected:'%v', but is '%v' ", expected, a)
	}
}

func checkTpl(t *testing.T, tplString string, data interface{}, expected string) {
	tpl := template.Must(template.New("template_name").Funcs(templateFuncMap).Parse(tplString))
	buf := new(bytes.Buffer)
	err := tpl.Execute(buf, data)
	if err != nil {
		t.Errorf("error executing  template '%s' with arg '%v' : %v", tplString, data, err)
	}
	if buf.String() != expected {
		t.Errorf("unexpected result for template '%s' : expected:'%s', but is '%s' ", tplString, expected, buf.String())
	}
}

func loadData(name string) interface{} {
	data := map[string]interface{}{}
	b, err := ioutil.ReadFile(filepath.Join("testdata/rv515/", name))
	if err != nil {
		panic("error reading file " + err.Error())
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.Decode(&data)
	return data
}
