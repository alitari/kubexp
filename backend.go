package kubexp

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"encoding/json"

	"net/http"

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
	cfg               *configType
	context           contextType
	resItems          map[string][]interface{}
	watches           map[string]*io.ReadCloser
	sorter            sorterType
	restExecutor      func(httpMethod, url string, body interface{}) (string, error)
	webSocketExecutor func(url string) (string, error)
	webSocketConnect  func(url string, closeCallback func()) (chan []byte, chan []byte, error)
	watchExecutor     func(url string) (*io.ReadCloser, error)
}

func newRestyBackend(cfg *configType, context contextType) *backendType {
	return &backendType{cfg: cfg, context: context,
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
		watchExecutor: func(url string) (*io.ReadCloser, error) {
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			}
			req, err := http.NewRequest("GET", url, nil)
			req.Header.Set("Authorization", "Bearer "+context.user.token)
			response, err := client.Do(req)
			if err != nil {
				errorlog.Printf("watch request error: %s", err)
				return nil, err
			}

			return &response.Body, nil
		},
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

func (b *backendType) resourceItems(ns string, resource resourceType) []interface{} {
	r := b.getList(ns, resource)
	switch r.(type) {
	case []interface{}:
		unsorted := r.([]interface{})
		b.sorter.setElements(unsorted)
		sort.Sort(b.sorter)
		ele := b.sorter.getElements()
		return ele
	}
	return nil
}

func (b *backendType) getNamespaces() ([]string, error) {
	res := b.cfg.resourcesOfName("namespaces")
	rc, err := b.restCallAll(res.APIPrefix, res.Name, nil)
	if err != nil {
		return nil, err
	}
	rcUn := unmarshall(rc)
	list := rcUn["items"].([]interface{})
	tracelog.Printf("nameSpaceList %s", list)
	nsList := make([]string, len(list))
	for i, ns := range list {
		nsList[i] = val(ns, []interface{}{"metadata", "name"}, "").(string)
	}
	return nsList, nil
}

func (b *backendType) createWatches() error {
	b.resItems = map[string][]interface{}{}
	b.watches = map[string]*io.ReadCloser{}
	nsList, err := b.getNamespaces()
	if err != nil {
		return err
	}
	tracelog.Printf("namespaces: %s", nsList)
	for _, res := range cfg.resources {
		if !res.Namespace {
			err = b.watch(res.APIPrefix, res.Name, "")
			if err != nil {
				return err
			}
		}
	}

	for _, ns := range nsList {
		for _, res := range cfg.resources {
			if res.Namespace {
				err = b.watch(res.APIPrefix, res.Name, ns)
				if err != nil {
					return err
				}
			}
		}
	}
	time.Sleep(1000 * time.Millisecond)
	return nil
}

func (b *backendType) closeWatches() {
	tracelog.Printf("close watches %s", b.watches)
	if b.watches != nil {
		for k, v := range b.watches {
			tracelog.Printf("Close watch %s", k)
			(*v).Close()
		}
	}
}

func (b *backendType) availabiltyCheck() error {
	_, err := b.restCallAll("api/v1", "nodes", nil)
	return err
}

func (b *backendType) getList(ns string, resource resourceType) interface{} {
	var k string
	if resource.Namespace {
		k = (&cachekeyType{ns, resource.Name, "", "list"}).key()
	} else {
		k = (&cachekeyType{"", resource.Name, "", "list"}).key()
	}
	return b.resItems[k]
}

// if b.cache[k] == nil {
// 	var rc string
// 	var err error
// 	if resource.Namespace {
// 		rc, err = b.restCall(http.MethodGet, resource.APIPrefix, resource.Name, ns, nil)
// 		if b.watches[k] == nil {
// 			body, err := b.watch(resource.APIPrefix, resource.Name, ns)
// 			if err != nil {
// 				errorlog.Printf("Return watch with error %v", err)
// 				return nil, err
// 			}
// 			b.watches[k] = body
// 		}
// 	} else {
// 		rc, err = b.restCallAll(resource.APIPrefix, resource.Name, nil)
// 		if b.watches[k] == nil {
// 			body, err := b.watchAll(resource.APIPrefix, resource.Name)
// 			if err != nil {
// 				errorlog.Printf("Return watch with error %v", err)
// 				return nil, err
// 			}
// 			b.watches[k] = body
// 			tracelog.Printf("set watch body k: %s, watches: %s, body: %s", k, b.watches, body)
// 		}
// 	}
// 	rcUn := unmarshall(rc)
// 	if err != nil {
// 		return rcUn, err
// 	}
// 	b.cache[k] = rcUn
// }

func (b *backendType) getDetail(ns string, resource resourceType, resourceItem string, view viewType) (interface{}, error) {
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
	return rcUn, nil
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
	return logs, nil
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
	return rp, nil
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

func (b *backendType) watch(apiPrefix, ress, ns string) error {
	k := (&cachekeyType{ns, ress, "", "list"}).key()
	if b.watches[k] != nil {
		return fmt.Errorf("duplicate watch. key: %s ", k)
	}
	tracelog.Printf("watching key: %s", k)
	var url string
	if ns != "" {
		url = fmt.Sprintf("%s/%s/watch/namespaces/%s/%s", b.context.Cluster.URL, apiPrefix, ns, ress)
	} else {
		url = fmt.Sprintf("%s/%s/watch/%s", b.context.Cluster.URL, apiPrefix, ress)
	}
	body, err := b.watchExecutor(url)
	if err != nil {
		errorlog.Printf("Return watch with error %v", err)
		return err
	}
	go func() {
		reader := bufio.NewReader(*body)
		for {
			//b := make([]byte, 0x10000)
			watchBytes, err := reader.ReadBytes('\n')
			if err != nil {
				if err.Error() == "EOF" || err.Error() == "use of closed network connection" {
					tracelog.Printf("Close watch url %s:\n reason: %v", url, err)
				} else {
					errorlog.Printf("Watch error url %s:\n error: %v", url, err)
				}
				break
			} else {
				b.updateResourceItems(k, watchBytes)
				if len(resourceMenu.widget.items) > 0 && len(namespaceList.widget.items) > 0 {
					selRes := currentResource()
					selNs := currentNamespace()
					if selNs == ns && selRes.Name == ress {
						updateResource()
					}
				}
			}
		}
	}()
	b.watches[k] = body
	return nil
}

func (b *backendType) updateResourceItems(k string, watchBytes []byte) {
	watch := unmarshallBytes(watchBytes)
	switch watch["type"] {
	case "MODIFIED":
		b.updateResourceItem(k, watch["object"].(map[string]interface{}))
	case "ADDED":
		b.addResourceItem(k, watch["object"].(map[string]interface{}))
	case "DELETED":
		b.deleteResourceItem(k, watch["object"].(map[string]interface{}))
	default:
		errorlog.Printf("unknown watch type : %s", watch["type"])
	}

	tracelog.Printf("resource items count k: %s , count: %d ", k, len(b.resItems[k]))
}

func (b *backendType) findResItem(k string, ri map[string]interface{}) int {
	riName := val(ri, []interface{}{"metadata", "name"}, "")
	items := b.resItems[k]
	for i, item := range items {
		if val(item, []interface{}{"metadata", "name"}, "") == riName {
			return i
		}
	}
	return -1
}

func (b *backendType) updateResourceItem(k string, ri map[string]interface{}) {
	tracelog.Printf("update k: %s", k)
	i := b.findResItem(k, ri)
	items := b.resItems[k]
	items[i] = ri
}

func (b *backendType) addResourceItem(k string, ri map[string]interface{}) {
	tracelog.Printf("add k: %s", k)
	items := b.resItems[k]
	b.resItems[k] = append(items, ri)
}

func (b *backendType) deleteResourceItem(k string, ri map[string]interface{}) {
	tracelog.Printf("delete k: %s", k)
	i := b.findResItem(k, ri)
	items := b.resItems[k]
	b.resItems[k] = append(items[:i], items[i+1:]...)
}

func unmarshallBytes(b []byte) map[string]interface{} {
	var dat map[string]interface{}
	if err := json.Unmarshal(b, &dat); err != nil {
		errorlog.Printf("can't unmarshall bytes '%s', error: %v", string(b), err)
	}
	return dat
}

func unmarshall(js string) map[string]interface{} {
	return unmarshallBytes([]byte(js))
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
