BUILDX_VER=v0.4.1
IMAGE_NAME=syepes/ping_exporter
VERSION?=latest

install:
	mkdir -vp ~/.docker/cli-plugins/ ~/dockercache
	curl --silent -L "https://github.com/docker/buildx/releases/download/${BUILDX_VER}/buildx-${BUILDX_VER}.linux-amd64" > ~/.docker/cli-plugins/docker-buildx
	chmod a+x ~/.docker/cli-plugins/docker-buildx

prepare: install
	docker buildx create --use

build-push:
	docker buildx build --push --platform linux/386,linux/amd64,linux/arm/v7,linux/arm64/v8,linux/s390x -t ${IMAGE_NAME}:${VERSION} .
