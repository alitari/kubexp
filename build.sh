 #!/bin/bash
./gitinfo.sh
if [ $# -eq 2 ] && [ $2 = "windows" ]; then
    executable="$1/kubeExplorer.exe"
    export GOOS=$2
else 
    executable="$1/kubeExplorer"
    export GOOS="linux"
fi
echo "building $GOOS  $executable ..."
go build -o $executable github.com/alitari/kubeExplorer/main
chmod a+x $executable
