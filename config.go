package kubexp

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

var kubeConfig string

var configFile *string

var useResourceFile = false

var contextColors = []string{"Magenta", "Cyan", "Blue"}

type configType struct {
	isNew      bool
	configFile string
	contexts   []contextType
	resources  []resourceType
}

type contextType struct {
	Name    string
	Cluster clusterType
	user    userType
	color   string
}

type clusterType struct {
	Name string
	URL  *url.URL
}

type userType struct {
	Name  string
	token string
}

type resourceType struct {
	Name, ShortName string
	Category        string
	APIPrefix       string
	Namespace       bool
	Watch           bool
	Views           []viewType
}

type viewType struct {
	Name     string
	Template string
}

var clusterResourcesTemplate = `{{ "capacity:" | contextColorEmp }} {{ .Capacity.String }}  {{ "cpu:" | contextColorEmp }} {{ .Requested.CPU.String }}({{ .PercentCPU }}) {{ "memory:" | contextColorEmp }} {{ .Requested.Memory.String }}({{ .PercentMem }})`

var helpTemplate = colorizeText(fmt.Sprintf(`
     _  __    _         ___          _                 
    | |/ /  _| |__  ___| __|_ ___ __| |___ _ _ ___ _ _ 
    | ' < || | '_ \/ -_) _|\ \ / '_ \ / _ \ '_/ -_) '_|
    |_|\_\_,_|_.__/\___|___/_\_\ .__/_\___/_| \___|_|  
                               |_|           %-6.6s
    Build: %s`, tag, commitID), 0, 259, cyanEmpInlineColor) + colorizeText(`

{{ "Context" | printf " %-15.15s " -}}
{{ "Key" | printf " %-16.16s " -}}
{{ "Command" | printf " %-30.30s " }}
__________________________________________________________________
{{- range . }}
{{ .KeyEvent.Viewname | ctx | printf " %-15.15s " -}}
{{ .KeyEvent | keyStr | printf " %-16.16s " -}}
{{ .Command.Name | printf " %-30.30s " -}}
{{ end }}`, 0, 180, yellowEmpInlineColor)

var errorTemplate = colorizeText(fmt.Sprintf(`
{{ (ind . 0)}}
Error:
{{ (ind . 1)}}`), 0, 180, redEmpInlineColor)

var confirmTemplate = colorizeText(fmt.Sprintf(`
{{ (ind . 0)}}
[Y]es or [N]o
`), 0, 180, yellowEmpInlineColor)

var loadingTemplate = colorizeText(fmt.Sprintf(`
{{ (ind . 0)}}
`), 0, 180, yellowEmpInlineColor)

var labelsAndAnnoTemplate = `{{ "Labels:" | whiteEmp }}` + mapTemplate(".metadata.labels") + `

{{ "Annotations:" | whiteEmp }}` + mapTemplate(".metadata.annotations")

var eventsTemplate = `
{{ "Events:" | whiteEmp }}
{{ $evs := (events .kind .metadata.name) }}
{{- if $evs }}
{{ "Age" | printf "%-8.8s" -}}
{{- "Reason" | printf "%-40.40s" -}}
{{- "Message" | printf "%s" }}
-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
{{range $ind, $event := $evs}}
{{- (age $event.metadata.creationTimestamp) | printf "%-8.8s" -}}
{{- $event.reason | printf "%-40.40s" -}}
{{- $event.message | printf "%s" }}
{{ end }}
{{- else -}}
No data
{{ end }}`

func mapTemplate(path string) string {
	return `
{{ $map := ` + path + `}}
{{- if $map -}}
{{range $k, $v := $map}}
{{- $k | printf "%-60.60s" -}}
{{- $v | printf "%-60.60s" }}
{{ end }}
{{- else -}}
No data
{{ end }}`
}

var yamlView = viewType{
	Name:     "yaml",
	Template: `{{- toYaml . -}}`,
}

var jsonView = viewType{
	Name:     "json",
	Template: `{{- toJSON . -}}`,
}

var infoView = viewType{
	Name:     "info",
	Template: fmt.Sprintf("%s\n%s", labelsAndAnnoTemplate, eventsTemplate),
}

var nameAgeColumns = `{{- header "Name" . .metadata.name | printf "%-50.50s " -}}
{{- header "Age" . (age .metadata.creationTimestamp) | printf "%-8.8s " -}}`

var statzusColumn = `{{- header "Name" . .metadata.name | printf "%-50.50s " -}}
{{- header "Age" . (age .metadata.creationTimestamp) | printf "%-8.8s " -}}`

var defaultResources = []resourceType{

	{Name: "nodes", APIPrefix: "api/v1", ShortName: "nodes", Category: "cluster/metadata", Namespace: false, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (fcwe .status.conditions "status" "True" "type") | printf "%-8.8s " -}}
` + nameAgeColumns + `
{{- header "Age" . (age .metadata.creationTimestamp) | printf "%-8.8s " -}}
{{- header "Version" . .status.nodeInfo.kubeletVersion | printf "%-10.10s " -}}
{{- header "Internal-IP" . ( fcwe .status.addresses "type" "InternalIP" "address") | printf "%-18.18s " -}}
{{- header "OS Image" . .status.nodeInfo.osImage | printf "%-25.25s " -}}
{{- header "Kernel Version" . .status.nodeInfo.kernelVersion | printf "%s " -}}`,
			},
			viewType{
				Name: "info",
				Template: `
{{ "Resources:" | whiteEmp }}
{{ nodeRes . }}

` + labelsAndAnnoTemplate + `

` + eventsTemplate,
			},
			yamlView,
			jsonView,
		}},
	{Name: "resourcequotas", APIPrefix: "api/v1", ShortName: "quota", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name:     "list",
				Template: nameAgeColumns,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "serviceaccounts", APIPrefix: "api/v1", ShortName: "sa", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Secrets" . (fc .secrets "name") | printf "%s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "componentstatuses", APIPrefix: "api/v1", ShortName: "cs", Category: "cluster/metadata", Namespace: false, Watch: false,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Status" . (fcwe .conditions "status" "True" "type" ) | printf "%-12.12s " -}}
{{- header "Message" . (fcwe .conditions "status" "True" "message" ) | printf "%-30.30s " -}}
{{- header "Error" . (fcwe .conditions "status" "True" "error" ) | printf "%s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "persistentvolumes", APIPrefix: "api/v1", ShortName: "pv", Category: "cluster/metadata", Namespace: false, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . ( .status.phase | blinkWhenChanged . "persistentvolumes" ) | printf "%-8.8s " -}}
` + nameAgeColumns + `
{{- header "Capacity" . .spec.capacity.storage | printf "%-12.12s " -}}
{{- header "Accessmodes" . ( printArray .spec.accessModes) | printf "%-20.20s " -}}
{{- header "Reclaimpolicy" . .spec.persistentVolumeReclaimPolicy | printf "%-15.15s " -}}

{{- header "Claim" . ( printf "%v/%v" .spec.claimRef.namespace .spec.claimRef.name ) | printf "%-30.30s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "events", APIPrefix: "api/v1", ShortName: "ev", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Resource" . .involvedObject.kind | printf "%-20.20s " -}}
{{- header "Res. name" . .involvedObject.name | printf "%-30.30s " -}}
{{- header "Reason" . .reason | printf "%-20.20s " -}}
{{- header "message" . .message | printf "%s" -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "limitranges", APIPrefix: "api/v1", ShortName: "limits", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name:     "list",
				Template: nameAgeColumns,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "podtemplates", APIPrefix: "api/v1", ShortName: "podtemplates", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name:     "list",
				Template: nameAgeColumns,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "endpoints", APIPrefix: "api/v1", ShortName: "ep", Category: "config/storage/discovery/loadbalancing", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name:     "list",
				Template: nameAgeColumns,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "services", APIPrefix: "api/v1", ShortName: "svc", Category: "config/storage/discovery/loadbalancing", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Cluster IP" . .spec.clusterIP | printf "%-18.18s " -}}
{{- header "External IP" . (printArray .spec.externalIPs) | printf "%-18.18s " -}}
{{- header "Ports" . (fc .spec.ports "port") | printf "%-15.15s " -}}
{{- header "TargetPorts" . (fc .spec.ports "targetPort") | printf "%-15.15s " -}}
{{- header "Selector" . ( printMap .spec.selector) | printf "%s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "configmaps", APIPrefix: "api/v1", ShortName: "cm", Category: "config/storage/discovery/loadbalancing", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Data" . ( keys .data) | printf "%s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "secrets", APIPrefix: "api/v1", ShortName: "secrets", Category: "config/storage/discovery/loadbalancing", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Type" . .type | printf "%-50.50s " -}}
{{- header "Data" . ( keys .data) | printf "%s " -}}`,
			},
			infoView,
			{
				Name: "decode",
				Template: `
Decoded secret values:
======================

{{range $key, $value := .data}}
{{ $key }}:
"{{ decode64 $value }}"

{{else}}
No Data 
{{end}}`,
			},
			yamlView,
			jsonView,
		}},
	{Name: "persistentvolumeclaims", APIPrefix: "api/v1", ShortName: "pvc", Category: "config/storage/discovery/loadbalancing", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Status" . .status.phase | printf "%-10.10s " -}}
{{- header "Volume" . .spec.volumeName | printf "%-30.30s " -}}
{{- header "Capacity" . .status.capacity.storage | printf "%-10.10s " -}}
{{- header "Access" . ( printArray .status.accessModes) | printf "%-16.16s " -}}
{{- header "Storageclass" . .spec.storageClassName | printf "%s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "pods", APIPrefix: "api/v1", ShortName: "po", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . ( podStatus .status | blinkWhenChanged . "pods" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `
{{- header "PortForward" . ( portForwardPortsShort . )|  printf "%-6.6s " -}}
{{- header "  R" . ( count ( fk .status.containerStatuses "ready" true )) | printf "%3.3s/" -}}
{{- header "A  " . (count .spec.containers) | printf "%-3.3s " -}}
{{- header "RC" . ( ind .status.containerStatuses 0).restartCount | printf "%-3.3s " -}}
{{- header "Reason" . (fcwe .status.conditions "status" "False" "reason") | printf "%-22.22s " -}}
{{- header "Restart" . .spec.restartPolicy | printf "%-10.10s " -}}
{{- header "Node" . .spec.nodeName | printf "%-22.22s " -}}
{{- header "Image" . ( ind .spec.containers 0).image | printf "%s " -}}
				`},
			viewType{
				Name: "info",
				Template: labelsAndAnnoTemplate + `

{{ "Port forward ports:" | whiteEmp }}
{{ portForwardPortsLong . }}

{{ "Resources:" | whiteEmp }}
{{ podRes . }}

` + eventsTemplate,
			},
			yamlView,
			jsonView,

			{
				Name:     "logs",
				Template: `{{- podLog -}}`,
			},
		}},
	{Name: "jobs", APIPrefix: "apis/batch/v1", ShortName: "jobs", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (( status .spec.completions .status.succeeded ) | blinkWhenChanged . "jobs" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `				
{{- header "Desired" . .spec.completions | printf "%8.8s " -}}
{{- header "Succ" . .status.succeeded | printf "%8.8s " -}}
{{- header "Fail" . .status.failed | printf "%8.8s   " -}}
{{- header "Container" . (( ind .spec.template.spec.containers 0).name) | printf "%-20.20s " -}}
{{- header "Image" . (( ind .spec.template.spec.containers 0).image) | printf "%-20.20s " -}}
{{- header "Selectors" . ( printMap .spec.selector.matchLabels) | printf "%s" -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "cronjobs", APIPrefix: "apis/batch/v1beta1", ShortName: "cronjobs", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (( status .spec.jobTemplate.spec.completions .status.succeeded ) | blinkWhenChanged . "cronjobs" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `
{{- header "Schedule" . .spec.schedule | printf "%-20.20s " -}}
{{- header "Last Scheduled" . .status.lastScheduleTime | printf "%-26.26s " -}}
{{- header "Container" . (( ind .spec.jobTemplate.spec.template.spec.containers 0).name) | printf "%-20.20s " -}}
{{- header "Image" . (( ind .spec.jobTemplate.spec.template.spec.containers 0).image) | printf "%-20.20s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "replicationcontrollers", APIPrefix: "api/v1", ShortName: "rc", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (( status .spec.replicas .status.readyReplicas ) | blinkWhenChanged . "replicationcontrollers" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `
{{- header "Desired" . .spec.replicas | printf "%8.8s " -}}
{{- header "Current" . .status.replicas | printf "%8.8s " -}}
{{- header "Ready" . .status.readyReplicas | printf "%8.8s   " -}}
{{- header "Container" . (( ind .spec.template.spec.containers 0).name) | printf "%-20.20s " -}}
{{- header "Image" . (( ind .spec.template.spec.containers 0).image) | printf "%-20.20s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "replicasets", APIPrefix: "apis/apps/v1", ShortName: "rs", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (( status .spec.replicas .status.readyReplicas ) | blinkWhenChanged . "replicasets" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `
{{- header "Desired" . .spec.replicas | printf "%8.8s " -}}
{{- header "Current" . .status.replicas | printf "%8.8s " -}}
{{- header "Ready" . .status.readyReplicas | printf "%8.8s   " -}}
{{- header "Container" . (( ind .spec.template.spec.containers 0).name) | printf "%-20.20s " -}}
{{- header "Image" . (( ind .spec.template.spec.containers 0).image) | printf "%s " -}}`,
			},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "namespaces", APIPrefix: "api/v1", ShortName: "ns", Category: "cluster/metadata", Namespace: false, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Status" . .status.phase | printf "%-12.12s " -}}
		   `},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "daemonsets", APIPrefix: "apis/apps/v1", ShortName: "ds", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (( status .status.desiredNumberScheduled .status.numberReady ) | blinkWhenChanged . "daemonsets" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `
{{- header "Desired" . .status.desiredNumberScheduled | printf "%8.8s " -}}
{{- header "Current" . .status.currentNumberScheduled | printf "%8.8s " -}}
{{- header "Ready" . .status.numberReady | printf "%8.8s   " -}}
{{- header "Updated" . .status.updatedNumberScheduled | printf "%8.8s   " -}}
{{- header "Available" . .status.numberAvailable | printf "%8.8s   " -}}
{{- header "Selectors" . ( printMap .spec.selector) | printf "%s " -}}
		   `},
			infoView,
			yamlView,
			jsonView,
		}},
	//NAME                                       DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
	{Name: "deployments", APIPrefix: "apis/apps/v1", ShortName: "deploy", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (( status .spec.replicas .status.readyReplicas ) | blinkWhenChanged . "deployments" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `
{{- header "Desired" . .spec.replicas | printf "%8.8s " -}}
{{- header "Current" . .status.replicas | printf "%8.8s " -}}
{{- header "Ready" . .status.readyReplicas | printf "%8.8s   " -}}
{{- header "Image" . (( ind .spec.template.spec.containers 0).image) | printf "%s " -}}
			`},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "statefulsets", APIPrefix: "apis/apps/v1", ShortName: "statefulsets", Category: "workloads", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: `{{- header "Status" . (( status .spec.replicas .status.readyReplicas ) | blinkWhenChanged . "statefulsets" ) | printf "%-10.10s " | colorPhase -}}
` + nameAgeColumns + `
{{- header "Desired" . .spec.replicas | printf "%8.8s " -}}
{{- header "Current" . .status.replicas | printf "%8.8s " -}}
{{- header "Ready" . .status.readyReplicas | printf "%8.8s   " -}}
{{- header "Image" . (( ind .spec.template.spec.containers 0).image) | printf "%-40.40s " -}}
{{- header "Selectors" . ( printMap .spec.selector) | printf "%s " -}}
			`},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "storageclasses", APIPrefix: "apis/storage.k8s.io/v1beta1", ShortName: "storagecl", Category: "cluster/metadata", Namespace: false, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Provisioner" . .provisioner | printf "%-30.30s " -}}
{{- header "Type" . .parameters.type | printf "%-20.20s " -}}
{{- header "Annotations" . ( printMap .metadata.annotations) | printf "%s " -}}
			`},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "poddisruptionbudgets", APIPrefix: "apis/policy/v1beta1", ShortName: "pdb", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Min-avail" . .spec.minAvailable | printf "%13.13s " -}}
{{- header "Allowed-disrupt" . .status.disruptionsAllowed | printf "%20.20s   " -}}
{{- header "Selectors" . ( printMap .spec.selector) | printf "%s " -}}
			`},
			infoView,
			yamlView,
			jsonView,
		}},
	//NAME           POD-SELECTOR   AGE
	{Name: "networkpolicies", APIPrefix: "apis/networking.k8s.io/v1", ShortName: "netpol", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Policy-types" . (printArray  .spec.policyTypes ) | printf "%-40.40s " -}}
{{- header "Pod-selector" . ( printMap .spec.podSelector) | printf "%s " -}}
			`},
			infoView,
			{
				Name: "rules",
				Template: labelsAndAnnoTemplate + `
{{ "Ingress:" | whiteEmp }}
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
{{range $i := .spec.ingress -}}
from
{{range $f := $i.from -}}
{{ if $f.ipBlock -}}
ip-Block         : {{ printMap $f.ipBlock }}
{{end -}}
{{ if $f.namespaceSelector -}}
namespaceSelector: {{ printMap $f.namespaceSelector }}
{{end -}}
{{ if $f.podSelector -}}
podSelector      : {{ printMap $f.podSelector }}
{{end -}}
{{end -}}
{{end }}

{{ "Egress:" | whiteEmp }}
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
{{range $e := .spec.egress -}}
ports: {{ printArray $e.ports }}  to: {{ printArray $e.to }} 
{{end -}}
`,
			},
			yamlView,
			jsonView,
		}},
	{Name: "ingresses", APIPrefix: "apis/extensions/v1beta1", ShortName: "ing", Category: "config/storage/discovery/loadbalancing", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Hosts" . (fc .spec.rules "host") | printf "%-40.40s " -}}
			`},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "horizontalpodautoscalers", APIPrefix: "apis/autoscaling/v1", ShortName: "hpa", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Reference" . .spec.scaleTargetRef.name | printf "%-40.40s " -}}
{{- header "CPU-Perc" . .spec.targetCPUUtilizationPercentage | printf "%12.12s " -}}
{{- header "Min-Pods" . .spec.minReplicas | printf "%12.12s " -}}
{{- header "Max-Pods" . .spec.maxReplicas | printf "%12.12s " -}}
{{- header "Replicas" . .status.currentReplicas | printf "%12.12s " -}}
			`},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "roles", APIPrefix: "apis/rbac.authorization.k8s.io/v1", ShortName: "roles", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Rules" . ( count .rules ) | printf "%-8.8s " -}}
{{- header "1stRule-API" . ( printArray (( ind .rules 0).apiGroups)) | printf "%-30.30s " -}}
{{- header "1stRule-resourceNames" . ( printArray (( ind .rules 0).resourceNames)) | printf "%-30.30s " -}}
{{- header "1stRule-resources" . ( printArray (( ind .rules 0).resources)) | printf "%-30.30s " -}}
{{- header "1stRule-verbs" . ( printArray (( ind .rules 0).verbs)) | printf "%-30.30s " -}}
			`},
			infoView,
			{
				Name: "rules",
				Template: `
{{ "Resources" | printf "%-90.90s " }} {{ "Verbs" | printf "%-90.90s " }}
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
{{range $rule := .rules}}
{{ printArray ($rule.resources) | printf "%-90.90s "}}  {{ printArray ($rule.verbs) | printf "%-90.90s "}}
{{end}}
`,
			},
			yamlView,
			jsonView,
		}},
	{Name: "rolebindings", APIPrefix: "apis/rbac.authorization.k8s.io/v1", ShortName: "rolebindings", Category: "namespace/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "RoleRef" . .roleRef.name | printf "%-45.45s " -}}
{{- header "subjects" . (fc .subjects "name" ) | printf "%s " -}}
			`},
			infoView,
			{
				Name: "bindings",
				Template: `
{{ "Role:" | whiteEmp }} {{ .roleRef.name }}
{{ "Subjects:" | whiteEmp }} 
{{"name"  | printf "%-40.40s "}}  {{"kind" | printf "%-40.40s "}}
---------------------------------------------------------------------------------
{{range $subject := .subjects -}}
{{ $subject.name | printf "%-40.40s "}}  {{ $subject.kind | printf "%-40.40s "}}
{{end}}
`,
			},
			yamlView,
			jsonView,
		}},
	{Name: "clusterroles", APIPrefix: "apis/rbac.authorization.k8s.io/v1", ShortName: "clusterroles", Category: "cluster/metadata", Namespace: false, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Rules" . ( count .rules ) | printf "%-8.8s " -}}
{{- header "1stRule-API" . ( printArray (( ind .rules 0).apiGroups)) | printf "%-30.30s " -}}
{{- header "1stRule-resourceNames" . ( printArray (( ind .rules 0).resourceNames)) | printf "%-30.30s " -}}
{{- header "1stRule-resources" . ( printArray (( ind .rules 0).resources)) | printf "%-30.30s " -}}
{{- header "1stRule-verbs" . ( printArray (( ind .rules 0).verbs)) | printf "%-30.30s " -}}
			`},
			infoView,
			{
				Name: "rules",
				Template: `
{{ "Resources" | printf "%-90.90s " }} {{ "Verbs" | printf "%-90.90s " }}
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
{{range $rule := .rules}}
{{ printArray ($rule.resources) | printf "%-90.90s "}}  {{ printArray ($rule.verbs) | printf "%-90.90s "}}
{{end}}
`,
			},
			yamlView,
			jsonView,
		}},
	{Name: "clusterrolebindings", APIPrefix: "apis/rbac.authorization.k8s.io/v1", ShortName: "clstrolebindings", Category: "cluster/metadata", Namespace: false, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "RoleRef" . .roleRef.name | printf "%-45.45s " -}}
{{- header "Subjects" . (fc .subjects "name" ) | printf "%s " -}}
			`},
			infoView,
			{
				Name: "bindings",
				Template: `
{{ "Role:" | whiteEmp }} {{ .roleRef.name }}
{{ "Subjects:" | whiteEmp }} 
{{"name"  | printf "%-40.40s "}}  {{"kind" | printf "%-40.40s "}}
---------------------------------------------------------------------------------
{{range $subject := .subjects -}}
{{ $subject.name | printf "%-40.40s "}}  {{ $subject.kind | printf "%-40.40s "}}
{{end}}
`,
			},
			yamlView,
			jsonView,
		}},
	{Name: "customresourcedefinitions", APIPrefix: "apis/apiextensions.k8s.io/v1beta1", ShortName: "customres", Category: "cluster/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name: "list",
				Template: nameAgeColumns + `
{{- header "Group" . .spec.group | printf "%-20.20s " -}}
{{- header "Kind" . .spec.names.kind | printf "%-20.20s " -}}
{{- header "Version" . .spec.version | printf "%-20.20s " -}}
			`},
			infoView,
			yamlView,
			jsonView,
		}},
	{Name: "controllerrevisions", APIPrefix: "apis/apps/v1", ShortName: "ctrlrevisions", Category: "cluster/metadata", Namespace: true, Watch: true,
		Views: []viewType{
			{
				Name:     "list",
				Template: nameAgeColumns,
			},
			infoView,
			yamlView,
			jsonView,
		}},
}

func newConfig(configFile string) *configType {
	return (&configType{configFile: configFile}).createContexts().createResources()
}

func (c *configType) resourcesOfCategory(category string) []resourceType {
	return c.resFilter(func(r resourceType) bool {
		return r.Category == category
	})
}

func (c *configType) resourcesOfName(name string) resourceType {
	return c.resFilter(func(r resourceType) bool {
		return r.Name == name
	})[0]
}

func (c *configType) resFilter(filter func(resourceType) bool) (ret []resourceType) {
	for _, r := range c.resources {
		if filter(r) {
			ret = append(ret, r)
		}
	}
	return ret
}

func (c *configType) listView(res resourceType) viewType {
	return c.resView(res, func(v viewType) bool {
		return v.Name == "list"
	})[0]
}

func (c *configType) detailViews(res resourceType) []viewType {
	return c.resView(res, func(v viewType) bool {
		return v.Name != "list"
	})
}

func (c *configType) detailView(res resourceType, name string) viewType {
	return c.resView(res, func(v viewType) bool {
		return v.Name == name
	})[0]
}

func (c *configType) resView(res resourceType, filter func(viewType) bool) (ret []viewType) {
	for _, v := range res.Views {
		if filter(v) {
			ret = append(ret, v)
		}
	}
	return ret
}

func (c *configType) allResourceCategories() []string {
	return []string{"cluster/metadata", "workloads", "config/storage/discovery/loadbalancing", "namespace/metadata"}
}

func (c *configType) createResources() *configType {
	if useResourceFile {
		p := filepath.Join(".", "resources.yaml")
		if _, err := os.Stat(p); os.IsNotExist(err) {
			d, err := yaml.Marshal(&defaultResources)
			s := string(d)
			tracelog.Printf("writing %s", s)
			if err != nil {
				errorlog.Fatalf("error: %v", err)
			}

			f, err := os.Create(p)
			if err != nil {
				errorlog.Fatalf("error: %v", err)
			}
			defer f.Close()
			_, err = f.Write(d)
			if err != nil {
				errorlog.Fatalf("error: %v", err)
			}
			infolog.Printf("saved resources file '%s'", f.Name())
		}
		resourcesData, err := ioutil.ReadFile(p)
		if err != nil {
			errorlog.Fatalf("error reading data from %v: %v", p, err)
		}

		var r []resourceType
		err = yaml.Unmarshal(resourcesData, &r)
		if err != nil {
			errorlog.Fatalf("Didn't understand the yaml in file %s: %v", p, err.Error())
		}
		c.resources = r
	} else {
		c.resources = defaultResources
	}
	return c
}

func (c *configType) createContexts() *configType {
	clustersData, err := ioutil.ReadFile(c.configFile)
	if err != nil {
		fatalStderrlog.Fatalf("Can't read file %s: %v", c.configFile, err.Error())
	}
	mess := fmt.Sprintf("reading clusters config from %s:", c.configFile)
	fmt.Println(mess)
	tracelog.Print(mess)

	var cfg map[string]interface{}
	err = yaml.Unmarshal(clustersData, &cfg)
	if err != nil {
		fatalStderrlog.Fatalf("Didn't understand the yaml in file %s: %v", c.configFile, err.Error())
	}
	contexts := cfg["contexts"].([]interface{})
	cs := make([]contextType, 0)
	for i, ctx := range contexts {
		cmap := ctx.(map[interface{}]interface{})
		cluster := c.parseCluster(cfg, cmap["context"])
		user := c.parseUser(cfg, cmap["context"])
		ct := contextType{Name: cmap["name"].(string), Cluster: cluster, user: user}
		mess := fmt.Sprintf("try connecting context '%s', cluster: '%s' ...", ct.Name, cluster.Name)
		fmt.Println(mess)
		infolog.Print(mess)
		var skip = false
		if ct.user.token == "<no value>" {
			fmt.Print("retrieving token from secret ...")
			infolog.Print("retrieving token from secret ...")
			err := retrieveContextToken(&ct)
			if err != nil {
				mess := fmt.Sprintf("Skipping context %s, due to error: %s", ct.Name, err.Error())
				fmt.Println(mess)
				warninglog.Print(mess)
				skip = true
			}
		}
		if !skip {
			err = c.isAvailable(ct)
			if err != nil {
				mess := fmt.Sprintf("Skipping context %s, due to error: %s", ct.Name, err.Error())
				fmt.Println(mess)
				warninglog.Print(mess)
				skip = true
			}
		}
		if !skip {
			colorIndex := len(cs) % 3
			ct.color = contextColors[colorIndex]
			cs = append(cs, ct)
			mess = fmt.Sprintf("created context no %d with name '%s' ", i+1, ct.Name)
			fmt.Println(mess)
			infolog.Print(mess)
		}
	}
	if len(cs) == 0 {
		fatalStderrlog.Fatalf("No contexts created for configfile '%s'. See logfile '%s' for details.", c.configFile, *logFilePath)
	}
	c.contexts = cs
	return c
}

func (c *configType) isAvailable(ct contextType) error {
	be := newBackend(ct)
	return be.availabiltyCheck()
}

func (c *configType) parseCluster(cfg map[string]interface{}, cm interface{}) clusterType {
	clusterName := val1(cm, "{{.cluster}}")
	if len(clusterName) == 0 {
		errorlog.Fatalf("No cluster found in context %v", cm)
	}
	cluster := filterArrayOnKeyValue(cfg["clusters"], "name", clusterName).([]interface{})[0]
	server := val1(cluster, "{{.cluster.server}}")
	if server == "" {
		errorlog.Fatalf("No server found in file %s", c.configFile)
	}
	url, err := (&url.URL{}).Parse(server)
	if err != nil {
		errorlog.Fatalf("error parsing url %v", err)
	}
	return clusterType{Name: clusterName, URL: url}
}

func (c *configType) parseUser(cfg map[string]interface{}, cm interface{}) userType {
	userName := val1(cm, "{{.user}}")
	if len(userName) == 0 {
		errorlog.Fatalf("No user found in context %v", cm)
	}
	user := filterArrayOnKeyValue(cfg["users"], "name", userName).([]interface{})[0]
	token := val1(user, "{{.user.token}}")
	return userType{Name: userName, token: token}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
