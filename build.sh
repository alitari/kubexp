 #!/bin/bash
./gitinfo.sh
if [ "$GOOS" = "windows" ]; then
    executable="$1/kubexp.exe"
else
    executable="$1/kubexp"
fi
GOOS=${GOOS:-"$(go env GOOS)"}
GOARCH=${GOARCH:-"$(go env GOARCH)"}
echo "building $GOOS $GOARCH $executable ..."
# go build -o $executable github.com/alitari/kubexp/main
go build -o $executable ./main
chmod a+x $executable
