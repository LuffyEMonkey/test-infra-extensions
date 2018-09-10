
REGISTRY="gcr.io/devenv-205606"

all: dockerize-linux launcher-linux sidecar-linux node-sidecar-linux

dockerize-build-linux:
	docker build -t $(REGISTRY)/dockerize-using-make:latest -f ./cmd/dockerize-using-make/Dockerfile ./cmd/dockerize-using-make
	docker build -t $(REGISTRY)/dockerize:latest -f ./cmd/dockerize/Dockerfile ./cmd/dockerize

launcher-build-linux:
	GOOS=linux GOARCH=amd64 go build -o cmd/pod-launcher/pod-launcher ./cmd/pod-launcher/main.go
	docker build -t $(REGISTRY)/pod-launcher:latest -f ./cmd/pod-launcher/Dockerfile ./cmd/pod-launcher

sidecar-build-linux:
	GOOS=linux GOARCH=amd64 go build -o cmd/sidecar/sidecar ./cmd/sidecar/main.go
	docker build -t $(REGISTRY)/sidecar:latest -f ./cmd/sidecar/Dockerfile ./cmd/sidecar

node-sidecar-build-linux:
	GOOS=linux GOARCH=amd64 go build -o cmd/node-sidecar/node-sidecar ./cmd/node-sidecar/main.go
	docker build -t $(REGISTRY)/node-sidecar:latest -f ./cmd/node-sidecar/Dockerfile ./cmd/node-sidecar

dockerize-linux: dockerize-build-linux
	docker push $(REGISTRY)/dockerize-using-make:latest
	docker push $(REGISTRY)/dockerize:latest

launcher-linux: launcher-build-linux
	docker push $(REGISTRY)/pod-launcher:latest

sidecar-linux: sidecar-build-linux
	docker push $(REGISTRY)/sidecar:latest

node-sidecar-linux: node-sidecar-build-linux
	docker push $(REGISTRY)/node-sidecar:latest
