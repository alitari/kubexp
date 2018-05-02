package kubexp

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strings"
	"time"

	"encoding/json"

	"net/http"
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

type backendType struct {
	cfg               *configType
	context           contextType
	resItems          map[string][]interface{}
	watches           map[string]*io.ReadCloser
	sorter            sorterType
	restExecutor      func(httpMethod, url, body string) (*http.Response, error)
	webSocketExecutor func(url string) (string, error)
	webSocketConnect  func(url string, closeCallback func()) (chan []byte, chan []byte, error)
}

func newRestyBackend(cfg *configType, context contextType) *backendType {
	return &backendType{cfg: cfg, context: context,
		restExecutor: func(httpMethod, url, body string) (*http.Response, error) {
			tracelog.Printf("rest call: %s %s", httpMethod, url)
			if body != "" {
				tracelog.Printf("body: '%s'", body)
			}
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
					IdleConnTimeout: 3 * time.Second,
				},
			}
			req, err := http.NewRequest(httpMethod, url, strings.NewReader(body))
			if err != nil {
				errorlog.Printf("can't create request url: %s, error: %v", url, err)
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+context.user.token)
			if httpMethod == http.MethodPatch {
				req.Header.Add("Content-Type", "application/strategic-merge-patch+json")
				req.Header.Add("Accept", "*/*")
			}

			response, err := client.Do(req)

			return response, err
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

func (b *backendType) resourceItems(ns string, resource resourceType) []interface{} {
	var r []interface{}
	if !resource.Namespace || ns != "" {
		r = b.resItems[getKey(ns, resource)]
	} else {
		for k, v := range b.resItems {
			if strings.Contains(k, resource.Name) {
				r = append(r, v...)
			}
		}
	}
	b.sorter.setElements(r)
	sort.Sort(b.sorter)
	ele := b.sorter.getElements()
	return ele
}

func (b *backendType) getNamespaces() ([]string, error) {
	res := b.cfg.resourcesOfName("namespaces")
	rc, err := b.restCallAll(res.APIPrefix, res.Name, "")
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
		if res.Watch {
			if !res.Namespace {
				err = b.watch(res.APIPrefix, res, "")
				if err != nil {
					return err
				}
			} else {
				for _, ns := range nsList {
					err = b.watch(res.APIPrefix, res, ns)
					if err != nil {
						return err
					}
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
	_, err := b.restCallAll("api/v1", "nodes", "")
	return err
}

func getKey(ns string, resource resourceType) string {
	if !resource.Namespace {
		ns = ""
	}
	return fmt.Sprintf("%s/%s", ns, resource.Name)
}

func (b *backendType) getDetail(ns string, resource resourceType, riName string) interface{} {
	return b.resItemByName(getKey(ns, resource), riName)
}

func (b *backendType) delete(ns string, resource resourceType, resourceItem string) (interface{}, error) {
	var rc string
	var err error
	if resource.Namespace {
		rc, err = b.restCall(http.MethodDelete, resource.APIPrefix, fmt.Sprintf("%s/%s", resource.Name, resourceItem), ns, "")
	} else {
		rc, err = b.restCallNoNs(http.MethodDelete, resource.APIPrefix, fmt.Sprintf("%s/%s", resource.Name, resourceItem), "")
	}
	if err != nil {
		return rc, err
	}
	return unmarshall(rc), nil
}

func (b *backendType) readPodLogs(ns, podName, containerName string) (interface{}, error) {
	logs, err := b.restCall(http.MethodGet, "api/v1", fmt.Sprintf("pods/%s/log?container=%s&tailLines=%v", podName, containerName, 1000), ns, "")
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
	depDetail, err := b.restCall(http.MethodGet, "apis/extensions/v1beta1", fmt.Sprintf("%s/%s", resource.Name, deploymentName), ns, "")
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

func (b *backendType) handleResponse(httpMethod, url, reqBody string, resp *http.Response, err error) (string, error) {
	if err != nil {
		mes := fmt.Sprintf("\nError: %v", err)
		errorlog.Print(mes)
		return mes, err
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		mes := fmt.Sprintf("\nError reading response: %v", err)
		errorlog.Print(mes)
		return mes, err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		tracelog.Printf("resources found for url: %s", url)
		return string(respBody), err
	case http.StatusNotFound:
		tracelog.Printf("no resources found for url: %s", url)
		return string(respBody), err
	default:
		mes := fmt.Sprintf("Request: %s '%s'\nBody: %s \nHTTP-Status: %d \nResponse: \n%s", httpMethod, url, reqBody, resp.StatusCode, string(respBody))
		errorlog.Printf(mes)
		return mes, errors.New(mes)
	}
}

func (b *backendType) restCallAll(apiPrefix, ress string, body string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", b.context.Cluster.URL, apiPrefix, ress)
	resp, err := b.restExecutor(http.MethodGet, url, body)
	return b.handleResponse(http.MethodGet, url, body, resp, err)
}

func (b *backendType) restCall(httpMethod, apiPrefix, ress, ns string, body string) (string, error) {
	url := fmt.Sprintf("%s/%s/namespaces/%s/%s", b.context.Cluster.URL, apiPrefix, ns, ress)
	resp, err := b.restExecutor(httpMethod, url, body)
	return b.handleResponse(httpMethod, url, body, resp, err)
}

func (b *backendType) restCallNoNs(httpMethod, apiPrefix, ress string, body string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", b.context.Cluster.URL, apiPrefix, ress)
	resp, err := b.restExecutor(httpMethod, url, body)
	return b.handleResponse(httpMethod, url, body, resp, err)
}

func (b *backendType) restCallBatch(httpMethod, ress string, ns string, body string) (string, error) {
	url := fmt.Sprintf("%s/apis/batch/v1/namespaces/%s/%s", b.context.Cluster.URL, ns, ress)
	resp, err := b.restExecutor(httpMethod, url, body)
	return b.handleResponse(httpMethod, url, body, resp, err)
}

func (b *backendType) watch(apiPrefix string, ress resourceType, ns string) error {
	k := getKey(ns, ress)
	if b.watches[k] != nil {
		return fmt.Errorf("duplicate watch. key: %s ", k)
	}
	tracelog.Printf("watching key: %s", k)
	var url string
	if ns != "" {
		url = fmt.Sprintf("%s/%s/watch/namespaces/%s/%s", b.context.Cluster.URL, apiPrefix, ns, ress.Name)
	} else {
		url = fmt.Sprintf("%s/%s/watch/%s", b.context.Cluster.URL, apiPrefix, ress.Name)
	}
	resp, err := b.restExecutor(http.MethodGet, url, "")
	if err != nil {
		errorlog.Printf("Return watch with error %v", err)
		return err
	}
	body := &resp.Body
	go func() {
		reader := bufio.NewReader(*body)
		for {
			//b := make([]byte, 0x10000)
			watchBytes, err := reader.ReadBytes('\n')
			if err != nil {
				if err.Error() == "EOF" || err.Error() == "use of closed network connection" {
					tracelog.Printf("Close watch url %s:\n reason: %v", url, err)
				} else {
					mess := fmt.Sprintf("Watch error url %s:\n error: %v", url, err)
					errorlog.Print(mess)
					showError(mess, err)
				}
				break
			} else {
				b.updateResourceItems(k, watchBytes)
				if len(resourceMenu.widget.items) > 0 && len(namespaceList.widget.items) > 0 {
					selRes := currentResource()
					selNs := currentNamespace()
					if selNs == ns && selRes.Name == ress.Name && currentState.name == "browseState" {
						updateResource()
					}
				}
			}
		}
	}()
	b.watches[k] = body
	return nil
}

func (b *backendType) indexOfResItemByName(k, name string) int {
	items := b.resItems[k]
	for i, item := range items {
		if resItemName(item) == name {
			return i
		}
	}
	return -1
}

func (b *backendType) resItemByName(k, name string) interface{} {
	items := b.resItems[k]
	for _, item := range items {
		if resItemName(item) == name {
			return item
		}
	}
	return nil
}

func (b *backendType) updateResourceItems(k string, watchBytes []byte) {
	watch := unmarshallBytes(watchBytes)
	watchObj := watch["object"].(map[string]interface{})
	switch watch["type"] {
	case "MODIFIED":
		b.updateResourceItem(k, watchObj)
	case "ADDED":
		b.addResourceItem(k, watchObj)
	case "DELETED":
		b.deleteResourceItem(k, watchObj)
	default:
		errorlog.Printf("unknown watch type , k: %s, watch: %s", k, watch)
	}

	//tracelog.Printf("resource items count k: %s , count: %d ", k, len(b.resItems[k]))
}

func (b *backendType) updateResourceItem(k string, ri map[string]interface{}) {
	name := resItemName(ri)
	//tracelog.Printf("update ri: (%s, %s)", k, name)
	i := b.indexOfResItemByName(k, name)
	if i > -1 {
		items := b.resItems[k]
		items[i] = ri
	} else {
		warninglog.Printf("update: ri not found k: %s , name: %s", k, name)
	}
}

func (b *backendType) addResourceItem(k string, ri map[string]interface{}) {
	//name := resItemName(ri)
	//tracelog.Printf("add ri: (%s, %s)", k, name)
	items := b.resItems[k]
	b.resItems[k] = append(items, ri)
}

func (b *backendType) deleteResourceItem(k string, ri map[string]interface{}) {
	name := resItemName(ri)
	tracelog.Printf("delete ri: (%s, %s)", k, name)
	i := b.indexOfResItemByName(k, name)
	if i > -1 {
		items := b.resItems[k]
		b.resItems[k] = append(items[:i], items[i+1:]...)
	} else {
		warninglog.Printf("delete: ri not found k: %s , name: %s", k, name)
	}
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
