all: image

code:
	glide install --strip-vendor --strip-vcs
	go install github.com/kopeio/eip-controller/cmd/eip-controller

test:
	go test -v github.com/kopeio/eip-controller/pkg/...

gofmt:
	gofmt -w -s cmd/
	gofmt -w -s pkg/

builder-image:
	docker build -f images/builder/Dockerfile -t builder .

build-in-docker: builder-image
	docker run -it -v `pwd`:/src builder /onbuild.sh

image: build-in-docker
	docker build -t kope/eip-controller  -f images/eip-controller/Dockerfile .

push: image
	docker push kope/eip-controller:latest
