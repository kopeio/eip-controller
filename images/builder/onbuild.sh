#!/bin/bash

mkdir -p /go
export GOPATH=/go

mkdir -p /go/src/github.com/kopeio/
ln -s /src /go/src/github.com/kopeio/eip-controller

cd /go/src/github.com/kopeio/eip-controller
/usr/bin/glide install --strip-vendor --strip-vcs

go install github.com/kopeio/eip-controller/cmd/eip-controller

mkdir -p /src/.build/artifacts/
cp /go/bin/eip-controller /src/.build/artifacts/
