.PHONY: build push

TAG=smartcontract/ethblockcomparer:1.0.1

build:
	docker build -t ${TAG} .

push:
	docker push ${TAG}
