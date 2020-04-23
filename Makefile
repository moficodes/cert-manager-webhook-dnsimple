IMAGE_NAME := "moficodes/cert-manager-webhook-dnsimple"
IMAGE_TAG := "v1.0.0"

OUT := $(shell pwd)/_out

$(shell mkdir -p "$(OUT)")

verify:
	sh ./scripts/fetch-test-binaries.sh
	go test -v .

build:
	DOCKER_BUILDKIT=1 docker build -t "$(IMAGE_NAME):$(IMAGE_TAG)" .

push:
	docker push "$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: rendered-manifest.yaml
rendered-manifest.yaml:
	helm template cert-manager-webhook-dnsimple \
        --set image.repository=$(IMAGE_NAME) \
        --set image.tag=$(IMAGE_TAG) \
        deploy/cert-manager-webhook-dnsimple > "$(OUT)/rendered-manifest.yaml"
