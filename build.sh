 #!/bin/bash
./gitinfo.sh
if [ $# -eq 2 ] && [ $2 = "windows" ]; then
    executable="$1/kubexp.exe"
    export GOOS=$2
else 
    executable="$1/kubexp"
    export GOOS="linux"
fi
echo "building $GOOS  $executable ..."
go build -o $executable github.com/alitari/kubexp/main
chmod a+x $executable
