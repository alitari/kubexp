package kubeExplorer

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"encoding/json"

	"net/http"

	"github.com/elgs/gojq"
	"gopkg.in/resty.v0"
)

type nameSorterType struct {
	elements  []interface{}
	ascending bool
}

func (s *nameSorterType) getName() string {
	return "NameSorter"
}

func (s *nameSorterType) getAscending() bool {
	return s.ascending
}

func (s *nameSorterType) setAscending(asc bool) {
	s.ascending = asc
}

func (s *nameSorterType) Len() int {
	return len(s.elements)
}

func (s *nameSorterType) Swap(i, j int) {
	s.elements[i], s.elements[j] = s.elements[j], s.elements[i]
}

func (s *nameSorterType) setElements(ele []interface{}) {
	s.elements = ele
}

func (s *nameSorterType) getElements() []interface{} {
	return s.elements
}

func (s *nameSorterType) Less(i, j int) bool {
	iName := val(s.elements[i], []interface{}{"metadata", "name"}, "").(string)
	jName := val(s.elements[j], []interface{}{"metadata", "name"}, "").(string)
	var res bool
	if len(iName) > 0 && len(jName) > 0 {
		res = strings.Compare(iName, jName) < 0
	} else {
		res = i < j
	}
	return (s.ascending && res) || (!s.ascending && !res)
}

type timeSorterType struct {
	elements  []interface{}
	ascending bool
}

func (s *timeSorterType) getName() string {
	return "AgeSorter"
}

func (s *timeSorterType) getAscending() bool {
	return s.ascending
}

func (s *timeSorterType) setAscending(asc bool) {
	s.ascending = asc
}

func (s *timeSorterType) Len() int {
	return len(s.elements)
}

func (s *timeSorterType) Swap(i, j int) {
	s.elements[i], s.elements[j] = s.elements[j], s.elements[i]
}

func (s *timeSorterType) setElements(ele []interface{}) {
	s.elements = ele
}

func (s *timeSorterType) getElements() []interface{} {
	return s.elements
}

func (s *timeSorterType) Less(i, j int) bool {
	itimeStr := val(s.elements[i], []interface{}{"metadata", "creationTimestamp"}, "").(string)
	jtimeStr := val(s.elements[j], []interface{}{"metadata", "creationTimestamp"}, "").(string)
	var res bool
	if len(itimeStr) > 0 && len(jtimeStr) > 0 {
		itime := totime(itimeStr)
		jtime := totime(jtimeStr)
		res = jtime.After(itime)
	} else {
		res = i < j
	}
	return (s.ascending && res) || (!s.ascending && !res)
}

type cachekeyType struct {
	ns           string
	resource     string
	resourceItem string
	context      string
}

func (v *cachekeyType) key() string {
	return fmt.Sprintf("%s-%s-%s-%s", v.ns, v.resource, v.resourceItem, v.context)
}

type backendType struct {
	context           contextType
	cache             map[string]interface{}
	sorter            sorterType
	restExecutor      func(httpMethod, url string, body interface{}) (string, error)
	webSocketExecutor func(url string) (string, error)
	webSocketConnect  func(url string, closeCallback func()) (chan []byte, chan []byte, error)
}

func newRestyBackend(context contextType) *backendType {
	return &backendType{context: context,
		restExecutor: func(httpMethod, url string, body interface{}) (string, error) {
			tracelog.Printf("rest call: %s %s", httpMethod, url)
			if body != nil {
				tracelog.Printf("body: '%v'", body)
			}
			resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
			resty.SetTimeout(time.Duration(3 * time.Second))
			rest := resty.R().SetHeader("Authorization", "Bearer "+context.user.token)

			var resp *resty.Response
			var err error
			switch httpMethod {
			case http.MethodGet:
				resp, err = rest.Get(url)
			case http.MethodDelete:
				resp, err = rest.Delete(url)
			case http.MethodPatch:
				rest := rest.SetHeader("Content-Type", "application/strategic-merge-patch+json").SetHeader("Accept", "*/*")
				resp, err = rest.SetBody(body).Patch(url)
			}
			if err != nil {
				mes := fmt.Sprintf("\nError: %v", err)
				errorlog.Print(mes)
				return mes, err
			}
			var rt string
			switch resp.StatusCode() {
			case http.StatusOK:
				rt = resp.String()
				tracelog.Printf("resources found for url: %s", url)
			case http.StatusNotFound:
				tracelog.Printf("no resources found for url: %s", url)
				rt = resp.String()
			default:
				mes := fmt.Sprintf("Request: %s '%s'\nBody: %v \nHTTP-Status: %d \nResponse: \n%s", httpMethod, url, body, resp.StatusCode(), resp.String())
				errorlog.Printf(mes)
				rt = mes
				err = errors.New(mes)
			}
			return rt, err
		},
		webSocketExecutor: func(url string) (string, error) {
			return websocketExecutor(url, context.user.token)
		},
		webSocketConnect: func(url string, closeCallback func()) (chan []byte, chan []byte, error) {
			return websocketConnect(url, context.user.token, closeCallback)
		},
		sorter: &nameSorterType{ascending: true},
	}
}

type sorterType interface {
	getName() string
	Len() int
	Less(i, j int) bool
	Swap(i, j int)
	setElements(ele []interface{})
	getElements() []interface{}
	setAscending(asc bool)
	getAscending() bool
}

func (b *backendType) resourceItems(ns string, resource resourceType) ([]interface{}, error) {
	r, err := b.getList(ns, resource)
	if err != nil {
		return nil, err
	}
	switch r.(type) {
	case map[string]interface{}:
		it := r.(map[string]interface{})
		its := it["items"]
		switch its.(type) {
		case ([]interface{}):
			unsorted := its.([]interface{})
			b.sorter.setElements(unsorted)
			sort.Sort(b.sorter)
			ele := b.sorter.getElements()
			return ele, nil
		}
	}
	return nil, nil
}

func (b *backendType) resetCache() {
	b.cache = map[string]interface{}{}
}

func (b *backendType) availabiltyCheck() error {
	_, err := b.restCallAll("api/v1", "nodes", nil)
	return err
}

func (b *backendType) getList(ns string, resource resourceType) (interface{}, error) {
	k := (&cachekeyType{ns, resource.Name, "", "list"}).key()
	if b.cache[k] == nil {
		var rc string
		var err error
		if resource.Namespace {
			rc, err = b.restCall(http.MethodGet, resource.APIPrefix, resource.Name, ns, nil)
		} else {
			rc, err = b.restCallAll(resource.APIPrefix, resource.Name, nil)
		}
		rcUn := unmarshall(rc)
		if err != nil {
			return rcUn, err
		}
		b.cache[k] = rcUn
	}
	return b.cache[k], nil
}

func (b *backendType) getDetail(ns string, resource resourceType, resourceItem string, view viewType) (interface{}, error) {
	k := (&cachekeyType{ns, resource.Name, resourceItem, view.Name}).key()
	if b.cache[k] == nil {
		var rc string
		var err error
		if resource.Namespace {
			rc, err = b.restCall(http.MethodGet, resource.APIPrefix, fmt.Sprintf("%s/%s", resource.Name, resourceItem), ns, nil)
		} else {
			rc, err = b.restCallNoNs(http.MethodGet, resource.APIPrefix, fmt.Sprintf("%s/%s", resource.Name, resourceItem), nil)
		}
		rcUn := unmarshall(rc)
		if err != nil {
			return rcUn, err
		}
		b.cache[k] = rcUn
	}
	return b.cache[k], nil
}

func (b *backendType) delete(ns string, resource resourceType, resourceItem string) (interface{}, error) {
	var rc string
	var err error
	if resource.Namespace {
		rc, err = b.restCall(http.MethodDelete, resource.APIPrefix, fmt.Sprintf("%s/%s", resource.Name, resourceItem), ns, nil)
	} else {
		rc, err = b.restCallNoNs(http.MethodDelete, resource.APIPrefix, fmt.Sprintf("%s/%s", resource.Name, resourceItem), nil)
	}
	if err != nil {
		return rc, err
	}
	return unmarshall(rc), nil
}

func (b *backendType) readPodLogs(ns, podName, containerName string) (interface{}, error) {
	k := (&cachekeyType{ns, "pods", podName + "/" + containerName, "readPodlogs"}).key()
	if b.cache[k] == nil {
		logs, err := b.restCall(http.MethodGet, "api/v1", fmt.Sprintf("pods/%s/log?container=%s&tailLines=%v", podName, containerName, 1000), ns, nil)
		if err != nil {
			return logs, err
		}
		logs = strings.Map(func(r rune) rune {
			if r == 0x1b || r == '\r' {
				return -1
			}
			return r
		}, logs)
		b.cache[k] = logs
	}
	return b.cache[k], nil
}

func (b *backendType) scale(ns string, resource resourceType, deploymentName string, scale int) (interface{}, error) {
	depDetail, err := b.restCall(http.MethodGet, "apis/extensions/v1beta1", fmt.Sprintf("%s/%s", resource.Name, deploymentName), ns, nil)
	if err != nil {
		return depDetail, err
	}
	tracelog.Printf("depDetal=%s", depDetail)
	spec := unmarshall(depDetail)["spec"]
	replicas := spec.(map[string]interface{})["replicas"]
	newReplicas := int(replicas.(float64)) + scale
	body := fmt.Sprintf(`{"spec":{ "replicas": %v }}`, newReplicas)
	r, err := b.restCall(http.MethodPatch, "apis/extensions/v1beta1", fmt.Sprintf("%s/%s", resource.Name, deploymentName), ns, body)
	if err != nil {
		return r, err
	}
	return r, nil
}

func (b *backendType) execPodCommand(namespace, podName, containerName, command string) (interface{}, error) {
	k := (&cachekeyType{namespace, "pods", podName + "/" + containerName, command}).key()
	if b.cache[k] == nil {
		var queryCommands bytes.Buffer
		for _, qc := range strings.Split(command, " ") {
			queryCommands.WriteString(fmt.Sprintf("command=%s&", qc))
		}
		stderr := "true"
		stdin := "true"
		stdout := "true"
		tty := "false"
		path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec?container=%s&%sstderr=%s&stdin=%s&stdout=%s&tty=%s", namespace, podName, containerName, queryCommands.String(), stderr, stdin, stdout, tty)
		url := fmt.Sprintf("%s://%s:%s%s", "wss", b.context.Cluster.URL.Hostname(), b.context.Cluster.URL.Port(), path)
		rp, err := b.webSocketExecutor(url)
		if err != nil {
			return rp, err
		}
		b.cache[k] = rp
	}
	return b.cache[k], nil
}

func (b *backendType) execIntoPod(namespace, podName, cmd, container string, closeCallback func()) (chan []byte, chan []byte, error) {
	stderr := "true"
	stdin := "true"
	stdout := "true"
	tty := "true"
	// command := "bin/sh"
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec?command=%s&container=%s&stderr=%s&stdin=%s&stdout=%s&tty=%s", namespace, podName, cmd, container, stderr, stdin, stdout, tty)
	url := fmt.Sprintf("%s://%s:%s%s", "wss", b.context.Cluster.URL.Hostname(), b.context.Cluster.URL.Port(), path)
	return b.webSocketConnect(url, closeCallback)
}

func (b *backendType) restCallAll(apiPrefix, ress string, body interface{}) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", b.context.Cluster.URL, apiPrefix, ress)
	return b.restExecutor(http.MethodGet, url, body)
}

func (b *backendType) restCall(httpMethod, apiPrefix, ress, ns string, body interface{}) (string, error) {
	url := fmt.Sprintf("%s/%s/namespaces/%s/%s", b.context.Cluster.URL, apiPrefix, ns, ress)
	return b.restExecutor(httpMethod, url, body)
}

func (b *backendType) restCallNoNs(httpMethod, apiPrefix, ress string, body interface{}) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", b.context.Cluster.URL, apiPrefix, ress)
	return b.restExecutor(httpMethod, url, body)
}

func (b *backendType) restCallBatch(httpMethod, ress string, ns string, body interface{}) (string, error) {
	url := fmt.Sprintf("%s/apis/batch/v1/namespaces/%s/%s", b.context.Cluster.URL, ns, ress)
	return b.restExecutor(httpMethod, url, body)
}

func parse0(js string, q string) interface{} {
	parser, err := gojq.NewStringQuery(js)
	v, err := parser.Query(q)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return v
}

func unmarshall(js string) map[string]interface{} {
	var dat map[string]interface{}
	if err := json.Unmarshal([]byte(js), &dat); err != nil {
		errorlog.Printf("can't unmarshall string '%s', error: %v", js, err)
	}
	return dat
}

func retrieveResourceStatus(conditions []interface{}) string {
	for i := 0; i < len(conditions); i++ {
		condition := conditions[i].(map[string]interface{})
		if condition["status"] == "True" {
			return condition["type"].(string)
		}
	}
	return "unknown"
}
