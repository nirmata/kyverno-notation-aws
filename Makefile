############
# DEFAULTS #
############

GIT_SHA              := $(shell git rev-parse HEAD)
REGISTRY             ?= ghcr.io
REPO                 ?= nirmata
IMAGE                ?= kyverno-notation-aws
GOOS                 ?= $(shell go env GOOS)
GOARCH               ?= $(shell go env GOARCH)
CGO_ENABLED          ?= 0
REPO_IMAGE           := $(REGISTRY)/$(REPO)/$(IMAGE)


#########
# TOOLS #
#########

TOOLS_DIR                          := $(PWD)/.tools
GO_ACC                             := $(TOOLS_DIR)/go-acc
GO_ACC_VERSION                     := latest
KO                                 := $(TOOLS_DIR)/ko
KO_VERSION                         := main #e93dbee8540f28c45ec9a2b8aec5ef8e43123966
HELM                               := $(TOOLS_DIR)/helm
HELM_VERSION                       := v3.12.3
TOOLS                              := $(GO_ACC) $(KO) $(HELM)

$(GO_ACC):
	@echo Install go-acc... >&2
	@GOBIN=$(TOOLS_DIR) go install github.com/ory/go-acc@$(GO_ACC_VERSION)

$(KO):
	@echo Install ko... >&2
	@GOBIN=$(TOOLS_DIR) go install github.com/google/ko@$(KO_VERSION)

$(HELM):
	@echo Install helm... >&2
	@GOBIN=$(TOOLS_DIR) go install helm.sh/helm/v3/cmd/helm@$(HELM_VERSION)

.PHONY: install-tools
install-tools: $(TOOLS) ## Install tools

.PHONY: clean-tools
clean-tools: ## Remove installed tools
	@echo Clean tools... >&2
	@rm -rf $(TOOLS_DIR)

##############
# UNIT TESTS #
##############

CODE_COVERAGE_FILE      := coverage
CODE_COVERAGE_FILE_TXT  := $(CODE_COVERAGE_FILE).txt
CODE_COVERAGE_FILE_HTML := $(CODE_COVERAGE_FILE).html

.PHONY: test
test: fmt vet test-clean test-unit ## Clean tests cache then run unit tests

.PHONY: fmt
fmt: ## Run go fmt
	@echo Go fmt... >&2
	@go fmt ./...

.PHONY: vet
vet: ## Run go vet
	@echo Go vet... >&2
	@go vet ./...

.PHONY: test-clean
test-clean: ## Clean tests cache
	@echo Clean test cache... >&2
	@go clean -testcache

.PHONY: test-unit
test-unit: test-clean $(GO_ACC) ## Run unit tests
	@echo Running unit tests... >&2
	@$(GO_ACC) ./... -o $(CODE_COVERAGE_FILE_TXT)

.PHONY: code-cov-report
code-cov-report: test-clean ## Generate code coverage report
	@echo Generating code coverage report... >&2
	@GO111MODULE=on go test -v -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out -o $(CODE_COVERAGE_FILE_TXT)
	@go tool cover -html=coverage.out -o $(CODE_COVERAGE_FILE_HTML)

################
# BUILD (LOCAL)#
################

CMD_DIR           := cmd
KYVERNO_DIR       := $(CMD_DIR)/kyverno
IMAGE_TAG_SHA     := $(GIT_SHA)
IMAGE_TAG_LATEST  := latest
PACKAGE           ?= github.com/nirmata/kyverno-notation-aws
ifdef VERSION
LD_FLAGS          := "-s -w -X $(PACKAGE)/pkg/version.BuildVersion=$(VERSION)"
else
LD_FLAGS          := "-s -w"
endif

build:
	go build -o kyverno-notation-aws

#################
# BUILD (DOCKER)#
#################

docker-build:
	@echo Build kyverno-notation-aws image with docker... >&2
	docker buildx create --name multiarch --driver docker-container --use
	docker buildx build --platform linux/amd64,linux/arm64/v8 -t $(REPO_IMAGE):$(IMAGE_TAG_LATEST) .
	docker buildx rm multiarch

docker-publish:
	@echo Build kyverno-notation-aws image with docker... >&2
	docker buildx create --name multiarch --driver docker-container --use
	docker buildx build --platform linux/amd64,linux/arm64/v8 -t $(REPO_IMAGE):$(IMAGE_TAG_LATEST) --push .
	docker tag $(REPO_IMAGE):$(IMAGE_TAG_LATEST) $(REPO_IMAGE):$(IMAGE_TAG_SHA)
	docker push $(REPO_IMAGE):$(IMAGE_TAG_SHA)
	docker push $(REPO_IMAGE):$(IMAGE_TAG_LATEST)
	docker buildx rm multiarch

########
# HELM #
########

.PHONY: codegen-helm-docs
codegen-helm-docs: ## Generate helm docs
	@echo Generate helm docs... >&2
	@docker run -v ${PWD}/charts:/work -w /work jnorwood/helm-docs:v1.11.0 -s file

.PHONY: install-kyverno-notation-aws
install-kyverno-notation-aws: $(HELM) ## Install kyverno notation AWS helm chart
	@echo Install kyverno chart... >&2
	@$(HELM) upgrade --install kyverno-notation-aws --namespace kyverno-notation-aws --create-namespace --wait ./charts/kyverno-notation-aws
