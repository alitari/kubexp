package kubexp

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"sort"
	"strconv"
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
	iName := fmt.Sprintf("%s/%s", resItemName(s.elements[i]), resItemNamespace(s.elements[i]))
	jName := fmt.Sprintf("%s/%s", resItemName(s.elements[j]), resItemNamespace(s.elements[j]))
	res := strings.Compare(iName, jName) < 0
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
	itimeStr := resItemCreationTimestamp(s.elements[i])
	jtimeStr := resItemCreationTimestamp(s.elements[j])
	var res bool
	if len(itimeStr) > 0 && len(jtimeStr) > 0 && jtimeStr != itimeStr {
		itime := totime(itimeStr)
		jtime := totime(jtimeStr)
		if itime.Equal(jtime) {
			res = true
		} else {
			res = jtime.After(itime)
		}
	} else {
		iName := fmt.Sprintf("%s/%s", resItemName(s.elements[i]), resItemNamespace(s.elements[i]))
		jName := fmt.Sprintf("%s/%s", resItemName(s.elements[j]), resItemNamespace(s.elements[j]))
		res = strings.Compare(iName, jName) < 0
	}
	return (s.ascending && res) || (!s.ascending && !res)
}

type changedObjType struct {
	id         string
	changeTime time.Time
}

type watchType struct {
	reader *io.ReadCloser
	online bool
}

type backendType struct {
	context             contextType
	resItems            map[string][]interface{}
	podLogs             []byte
	watches             map[string]*watchType
	sorter              sorterType
	restExecutor        func(httpMethod, url, body string, timout int) (*http.Response, error)
	updateLoop          <-chan time.Time
	lastLivenessCheck   time.Time
	clusterLivenessDone chan bool
	changedObjList      []changedObjType
	changedObjSet       map[string]bool
	blink               bool
}

func newBackend(context contextType) *backendType {
	return &backendType{context: context,
		restExecutor: func(httpMethod, url, body string, timeout int) (*http.Response, error) {
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
			if timeout > 0 {
				client.Timeout = time.Duration(timeout) * time.Second
			}
			req, err := http.NewRequest(httpMethod, url, strings.NewReader(body))
			if err != nil {
				errorlog.Printf("can't create request url: %s, error: %v", url, err)
				return nil, err
			}
			if !strings.HasPrefix(url, "http://127.0.0.1") && !strings.HasPrefix(url, "http://localhost") {
				req.Header.Set("Authorization", "Bearer "+context.user.token)
			}
			if httpMethod == http.MethodPatch {
				req.Header.Add("Content-Type", "application/strategic-merge-patch+json")
				req.Header.Add("Accept", "*/*")
			}

			response, err := client.Do(req)
			if err == nil {
				tracelog.Printf("rest call: %s %s , response status: %s", httpMethod, url, response.Status)
			}

			return response, err
		},

		sorter:              &nameSorterType{ascending: true},
		updateLoop:          time.NewTicker(time.Duration(250) * time.Millisecond).C,
		clusterLivenessDone: make(chan bool),
		changedObjList:      []changedObjType{},
		changedObjSet:       map[string]bool{},
		blink:               true,
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

func (b *backendType) resourceItems(ns string, rt resourceType) []interface{} {
	var r []interface{}
	if rt.Namespace && ns != "*ALL*" {
		for _, ri := range b.resItems[rt.Name] {
			if resItemNamespace(ri) == ns {
				r = append(r, ri)
			}
		}
	} else {
		r = b.resItems[rt.Name]
	}
	b.sorter.setElements(r)
	sort.Sort(b.sorter)
	ele := b.sorter.getElements()
	return ele
}

func (b *backendType) createWatches(resources []resourceType) error {
	b.resItems = map[string][]interface{}{}
	b.watches = map[string]*watchType{}
	for _, res := range resources {
		if res.Watch {
			err := b.watch(res.APIPrefix, res.Name)
			if err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	go func() {
		for {
			select {
			case <-b.updateLoop:
				now := time.Now()
				if (now.Sub(b.lastLivenessCheck)).Seconds() > 10 {
					b.lastLivenessCheck = now
					err := b.availabiltyCheck()
					if err != nil {
						for _, w := range b.watches {
							w.online = false
						}
						errorlog.Printf("cluster heart beat not ok: %v", err)
					}
				}

				b.updateChangedObjList()
				b.blink = !b.blink

				if currentState.name == "browseState" {
					updateResourceItemList(false)
				}

			case <-b.clusterLivenessDone:
				infolog.Printf("clusterHeartbeatDone")
				return
			}
		}
	}()

	return nil
}

func (b *backendType) updateChangedObjList() {
	var k = 0
	for i, el := range b.changedObjList {
		chgTime := el.changeTime
		if time.Now().Sub(chgTime).Seconds() < 5 {
			k = i
			break
		}

	}
	newList := b.changedObjList[k:]
	for _, el := range b.changedObjList[:k] {
		b.changedObjSet[el.id] = false
	}

	b.changedObjList = newList
}

func (b *backendType) closeWatches0(filter func(string) bool) {
	tracelog.Printf("close watches %s", b.watches)
	if b.watches != nil {
		for k, v := range b.watches {
			if filter(k) {
				tracelog.Printf("Close watch %s", k)
				(*v.reader).Close()
			}
		}
	}
}

func (b *backendType) closeWatches() {
	b.closeWatches0(func(s string) bool {
		return true
	})
	b.clusterLivenessDone <- true
}

func (b *backendType) delete(ns string, resource resourceType, resourceItem string, noGracePeriod bool) (interface{}, error) {
	var rc string
	var err error
	queryParam := ""
	if noGracePeriod {
		queryParam = "?gracePeriodSeconds=0"
	}
	if resource.Namespace {
		rc, err = b.restCall(http.MethodDelete, resource.APIPrefix, fmt.Sprintf("%s/%s%s", resource.Name, resourceItem, queryParam), ns, "")
	} else {
		rc, err = b.restCallNoNs(http.MethodDelete, resource.APIPrefix, fmt.Sprintf("%s/%s%s", resource.Name, resourceItem, queryParam), "")
	}
	if err != nil {
		return rc, err
	}
	return unmarshall(rc), nil
}

func (b *backendType) scale(ns string, resource resourceType, deploymentName string, spec interface{}, scale int) (interface{}, error) {
	replicas, _ := strconv.Atoi(val1(spec, "{{.spec.replicas}}"))
	newReplicas := replicas + scale
	body := fmt.Sprintf(`{"spec":{ "replicas": %v }}`, newReplicas)
	r, err := b.restCall(http.MethodPatch, resource.APIPrefix, fmt.Sprintf("%s/%s", resource.Name, deploymentName), ns, body)
	if err != nil {
		return r, err
	}
	return r, nil
}

func (b *backendType) handleResponse(httpMethod, url, reqBody string, resp *http.Response, err error) (string, error) {
	if err != nil {
		mes := fmt.Sprintf("\nError calling '%s %s %s'\ndetails: %s", httpMethod, url, reqBody, err)
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

func (b *backendType) availabiltyCheck() error {
	url := fmt.Sprintf("%s/%s/%s", b.context.Cluster.URL, "api/v1", "namespaces")
	resp, err := b.restExecutor(http.MethodGet, url, "", restCallTimeout)
	_, err = b.handleResponse(http.MethodGet, url, "", resp, err)
	return err
}

func (b *backendType) restCallAll(apiPrefix, ress string, body string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", b.context.Cluster.URL, apiPrefix, ress)
	resp, err := b.restExecutor(http.MethodGet, url, body, restCallTimeout)
	return b.handleResponse(http.MethodGet, url, body, resp, err)
}

func (b *backendType) restCall(httpMethod, apiPrefix, ress, ns string, body string) (string, error) {
	url := fmt.Sprintf("%s/%s/namespaces/%s/%s", b.context.Cluster.URL, apiPrefix, ns, ress)
	resp, err := b.restExecutor(httpMethod, url, body, restCallTimeout)
	return b.handleResponse(httpMethod, url, body, resp, err)
}

func (b *backendType) restCallNoNs(httpMethod, apiPrefix, ress string, body string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", b.context.Cluster.URL, apiPrefix, ress)
	resp, err := b.restExecutor(httpMethod, url, body, restCallTimeout)
	return b.handleResponse(httpMethod, url, body, resp, err)
}

func (b *backendType) restCallBatch(httpMethod, ress string, ns string, body string) (string, error) {
	url := fmt.Sprintf("%s/apis/batch/v1/namespaces/%s/%s", b.context.Cluster.URL, ns, ress)
	resp, err := b.restExecutor(httpMethod, url, body, restCallTimeout)
	return b.handleResponse(httpMethod, url, body, resp, err)
}

// GET /api/v1/watch/pods
func (b *backendType) watch(apiPrefix string, resName string) error {
	urlPrefix := fmt.Sprintf("%s/%s/watch", b.context.Cluster.URL, apiPrefix)
	return b.watch0(urlPrefix, resName, "")
}

// GET /api/v1/namespaces/{namespace}/pods/{name}/log
func (b *backendType) watchPodLogs(ns, podName, containerName string) error {
	b.podLogs = []byte{}
	urlPrefix := fmt.Sprintf("%s/%s/namespaces/%s", b.context.Cluster.URL, "api/v1", ns)
	urlPostfix := fmt.Sprintf("pods/%s/log", podName)
	queryParam := fmt.Sprintf("?tailLines=%v&follow=true&container=%s", 1000, containerName)
	return b.watch0(urlPrefix, urlPostfix, queryParam)
}

func (b *backendType) closePodLogsWatch() {
	b.closeWatches0(func(s string) bool {
		return strings.HasSuffix(s, "log")
	})
}

func (b *backendType) watch0(urlPrefix, urlPostfix, queryParam string) error {
	tracelog.Printf("watching resource : %s", urlPostfix)
	url := fmt.Sprintf("%s/%s%s", urlPrefix, urlPostfix, queryParam)
	resp, err := b.restExecutor(http.MethodGet, url, "", -1)
	if err != nil {
		errorlog.Printf("error watching resource %s : %v", urlPostfix, err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		mess := fmt.Sprintf("error watching resource %s: http status %s", urlPostfix, resp.Status)
		if resp.StatusCode == http.StatusUnauthorized {
			mess = mess + "\nPlease check your cluster rbac settings!"
		}
		errorlog.Printf(mess)
		return fmt.Errorf(mess)
	}
	body := &resp.Body
	go func() {
		reader := bufio.NewReader(*body)
		for {
			watchBytes, err := reader.ReadBytes('\n')
			if err != nil {
				if err.Error() == "EOF" || strings.Contains(err.Error(), "use of closed network connection") {
					tracelog.Printf("Close watch url %s:\n reason: %v", url, err)

				} else {
					mess := fmt.Sprintf("Watch error url %s:\n error: %v", url, err)
					errorlog.Print(mess)
					showError(mess, err)
				}
				b.watches[urlPostfix].online = false
				break
			} else {
				b.watches[urlPostfix].online = true
				if !strings.HasSuffix(urlPostfix, "log") {
					b.updateResourceItems(urlPostfix, watchBytes)
				} else {
					b.podLogs = append(b.podLogs, watchBytes[:]...)
					updateResourceItemDetailPart()
				}
			}
		}
	}()

	b.watches[urlPostfix] = &watchType{reader: body, online: false}
	return nil
}

func (b *backendType) indexOfResItemByName(resName, name string) int {
	items := b.resItems[resName]
	for i, item := range items {
		if resItemName(item) == name {
			return i
		}
	}
	return -1
}

func (b *backendType) updateResourceItems(resName string, watchBytes []byte) {
	watch := unmarshallBytes(watchBytes)
	if watch["object"] != nil {
		watchObj := watch["object"].(map[string]interface{})
		switch watch["type"] {
		case "MODIFIED":
			b.updateResourceItem(resName, watchObj)
		case "ADDED":
			b.addResourceItem(resName, watchObj)
		case "DELETED":
			b.deleteResourceItem(resName, watchObj)
		default:
			errorlog.Printf("unknown watch type , resource: %s, watch: %s", resName, watch)
		}
		if resName == "namespaces" {
			updateNamespaces()
		}
		chngObj := changedObjType{id: fmt.Sprintf("%s/%s/%s", resItemNamespace(watchObj), resName, resItemName(watchObj)), changeTime: time.Now()}

		b.changedObjList = append(b.changedObjList, chngObj)
		b.changedObjSet[chngObj.id] = true

		if len(resourceMenu.widget.items) > 0 && len(namespaceList.widget.items) > 0 {
			selRes := selectedResource()
			selNs := selectedNamespace()

			if currentState.name == "browseState" && selRes.Name == resName && (selNs == "*ALL*" || selNs == resItemNamespace(watchObj)) && (watch["type"] == "DELETED" || watch["type"] == "ADDED") {
				updateResourceItemList(true)
			}
		}
	} else {
		errorlog.Printf("unknown watch obj: %v", watch)
	}

	//tracelog.Printf("count of %s:  : %d ", resName, len(b.resItems[resName]))
}

func (b *backendType) updateResourceItem(resName string, ri map[string]interface{}) {
	name := resItemName(ri)
	// tracelog.Printf("update ri: (%s, %s)", resName, name)
	i := b.indexOfResItemByName(resName, name)
	if i > -1 {
		items := b.resItems[resName]
		items[i] = ri
	} else {
		warninglog.Printf("update: ri  (%s/%s) not found", resName, name)
	}
}

func (b *backendType) addResourceItem(resName string, ri map[string]interface{}) {
	// name := resItemName(ri)
	// tracelog.Printf("add ri: (%s, %s)", resName, name)
	items := b.resItems[resName]
	b.resItems[resName] = append(items, ri)
}

func (b *backendType) deleteResourceItem(resName string, ri map[string]interface{}) {
	name := resItemName(ri)
	tracelog.Printf("delete ri (%s/%s)", resName, name)
	i := b.indexOfResItemByName(resName, name)
	if i > -1 {
		items := b.resItems[resName]
		b.resItems[resName] = append(items[:i], items[i+1:]...)
	} else {
		warninglog.Printf("delete: ri (%s/%s) not found ", resName, name)
	}
}

func kubectl(ctx string, a1 string, a ...string) *exec.Cmd {
	context := fmt.Sprintf("--context=%s", ctx)
	timeout := fmt.Sprintf("--request-timeout=%ds", kubeCtlTimeout)
	full := append([]string{context, timeout, a1}, a[:]...)
	tracelog.Printf("kubectl %v", full)
	return execCommand("kubectl", full...)
}

func runCmd(cmd *exec.Cmd) (string, string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()

	return out.String(), stderr.String(), err
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
