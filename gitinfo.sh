 #!/bin/bash
COMMIT_ID=$(git rev-parse HEAD)
BRANCH=$(git branch | grep \* | cut -d ' ' -f2)
TAG=$(git describe --tags --abbrev=0)
(printf 'package kubeExplorer\nvar commitID = "%s" \n' "$COMMIT_ID") > gitInfo.go
(printf 'var branch = "%s" \n' "$BRANCH") >> gitInfo.go
(printf 'var tag = "%s" \n' "$TAG") >> gitInfo.go
