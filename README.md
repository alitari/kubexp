# kubexp (KubeExplorer)

[![CircleCI](https://circleci.com/gh/alitari/kubexp.svg?style=svg&circle-token=0a1cb7c84884d737a8f742e7775ef88dbda65aff)](https://circleci.com/gh/alitari/kubexp)


kubexp is a console user interface for [kubernetes](https://kubernetes.io/). The main purpose of this tool is to enable a fast and efficient access to kubernetes cluster resources. I recommend not to use it on production clusters. You need to know that this tool avoids confirmation dialogs even for irreversible commands like delete.

## Setup

### rbac

Your [service account](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/) must have a rolebinding to cluster admin in each k8s cluster. The file [rbac-default-clusteradmin.yaml](./rbac-default-clusteradmin.yaml) contains the according [clusterrolebinding]((https://kubernetes.io/docs/admin/authorization/rbac/#kubectl-create-clusterrolebinding)) for the default service account:

```bash
kubectl apply -f rbac-default-clusteradmin.yaml
```

### configure clusters

kubexp uses `~/.kube/config` to read the k8s contexts. The user of a context *must* have a token defined:

```yaml
apiVersion: v1
clusters:
- cluster: ...
...
contexts:
- context:
    cluster: ...
    user: kubernetes-admin
  name: my-context
users:
- name: kubernetes-admin
  user:
    # we need the line below to include `my-context`
    token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9....
...
```

Having access to your cluster with [`kubectl`](https://kubernetes.io/docs/user-guide/kubectl-overview/) you can add the token to your current context:

```bash
TOKEN=$(kubectl describe secret $(kubectl get secrets | grep default | cut -f1 -d ' ') | grep -E '^token' | cut -f2 -d':' | tr -d '\t')
KUBE_USER=$(kubectl config get-contexts | grep "*" | awk -v N=4 '{print $N}')
kubectl config set-credentials $KUBE_USER --token="$TOKEN"
```

### get executable

Go to [releases page](https://github.com/alitari/kubexp/releases) and download the binary for your platform: 

### command line options

Call `kubexp -help`

### first steps

Once the ui is up, you can press `h` for help.


## build from source

### get dependencies

```bash
go get -v -t -d ./...
```

### building

- windows

```bash
./build.sh bin windows
```

- linux

```bash
./build.sh bin linux
```

### running

- run tests:

```bash
go test main/..
```

- under windows:

```bash
bin/kubexp.exe
```

- under linux:

```bash
bin/kubexp
```
