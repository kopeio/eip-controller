#!/bin/bash

mkdir -p /go
export GOPATH=/go

mkdir -p /go/src/github.com/kopeio/
ln -s /src /go/src/github.com/kopeio/eip-controller

cd /go/src/github.com/kopeio/eip-controller

make code

mkdir -p /src/.build/artifacts/
cp /go/bin/eip-controller /src/.build/artifacts/
