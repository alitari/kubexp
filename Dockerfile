FROM golang:1.8

RUN apt-get update && apt-get install -y apt-transport-https
RUN curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
RUN echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list

RUN apt-get update && apt-get install -y kubectl

WORKDIR /go/src/github.com/alitari/kubexp/
COPY . .

RUN go get -d -v ./...
ENV GOOS="linux"
RUN ./build.sh bin


ENTRYPOINT ["/go/src/github.com/alitari/kubexp/bin/kubexp"]

