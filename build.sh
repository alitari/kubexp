 #!/bin/bash
./gitinfo.sh
if [ $GOOS = "windows" ]; then
    executable="$1/kubexp.exe"
else 
    executable="$1/kubexp"
fi
export GOARCH=amd64
echo "building $GOARCH $GOOS  $executable ..."
# go build -o $executable github.com/alitari/kubexp/main
go build -o $executable ./main
chmod a+x $executable
