BUILDX_VER=v0.10.2
IMAGE_NAME=syepes/network_exporter
VERSION?=latest

install:
	mkdir -vp ~/.docker/cli-plugins/ ~/dockercache
	curl --silent -L "https://github.com/docker/buildx/releases/download/${BUILDX_VER}/buildx-${BUILDX_VER}.linux-amd64" > ~/.docker/cli-plugins/docker-buildx
	chmod a+x ~/.docker/cli-plugins/docker-buildx

prepare: install
	docker buildx create --use

build:
	docker buildx build --platform linux/amd64,linux/arm/v7,linux/arm64/v8 -t ${IMAGE_NAME}:${VERSION} .

build-push:
	docker buildx build --push --platform linux/amd64,linux/arm/v7,linux/arm64/v8 -t ${IMAGE_NAME}:${VERSION} .

build-local:
	goreleaser release --skip-publish --snapshot --rm-dist

lint:
	golangci-lint run
