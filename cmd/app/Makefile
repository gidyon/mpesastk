# image := 201661467817.dkr.ecr.eu-north-1.amazonaws.com/mpesa-stk
image := public.ecr.aws/q1f9b5m5/mpesa-stk
context := .

ifdef IMAGE
	image=$(IMAGE)
else
	imagex := $(image)
	ifdef tag
		image=$(imagex):$(tag)
	else
		image=$(imagex):latest
	endif
endif

ifdef BUILD_CONTEXT
	context=$(BUILD_CONTEXT)
endif

run:
	go build -v -o service && ./service -config-file=./.env

gotest:
	@cd $(PROJECT_ROOT)/internal/mpesapayment && ginkgo -cover
	
compile:
	@GOOS=linux CGO_ENABLED=0 go build -tags netgo -installsuffix netgo -v -o service .

docker_build:
	@docker build -t $(image) .

docker_tag:
	@docker tag $(image) $(image)

get_auth:
	aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin 201661467817.dkr.ecr.eu-north-1.amazonaws.com

docker_push:
	@docker push $(image)

build_service: compile docker_build docker_tag get_auth docker_push

build_service_no_auth: compile docker_build docker_tag docker_push

deploy:
	@kubectl delete -f deploy.yaml && kubectl apply -f deploy.yaml

build_and_deploy: gotest compile docker_build docker_tag docker_push deploy



