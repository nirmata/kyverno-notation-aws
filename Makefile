############
# DEFAULTS #
############

GIT_SHA              := $(shell git rev-parse HEAD)
REGISTRY             ?= ghcr.io
REPO                 ?= nirmata
IMAGE                ?= kyverno-notation-aws
GOOS                 ?= $(shell go env GOOS)
GOARCH               ?= $(shell go env GOARCH)
REPO_IMAGE           := $(REGISTRY)/$(REPO)/$(IMAGE)


#########
# TOOLS #
#########

TOOLS_DIR                          := $(PWD)/.tools
GO_ACC                             := $(TOOLS_DIR)/go-acc
GO_ACC_VERSION                     := latest
KO                                 := $(TOOLS_DIR)/ko
KO_VERSION                         := main #e93dbee8540f28c45ec9a2b8aec5ef8e43123966
TOOLS                              := $(GO_ACC) $(KO)

$(GO_ACC):
	@echo Install go-acc... >&2
	@GOBIN=$(TOOLS_DIR) go install github.com/ory/go-acc@$(GO_ACC_VERSION)

$(KO):
	@echo Install ko... >&2
	@GOBIN=$(TOOLS_DIR) go install github.com/google/ko@$(KO_VERSION)

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
test: test-clean test-unit ## Clean tests cache then run unit tests

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


CMD_DIR        := cmd
KYVERNO_DIR    := $(CMD_DIR)/kyverno
PACKAGE        ?= github.com/nirmata/kyverno-notation-aws
ifdef VERSION
LD_FLAGS       := "-s -w -X $(PACKAGE)/pkg/version.BuildVersion=$(VERSION)"
else
LD_FLAGS       := "-s -w"
endif

.PHONY: fmt
fmt: ## Run go fmt
	@echo Go fmt... >&2
	@go fmt ./...

.PHONY: vet
vet: ## Run go vet
	@echo Go vet... >&2
	@go vet ./...

build:
	go build -o kyverno-notation-aws

docker:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o kyverno-notation-aws .
	docker buildx build --platform linux/arm64/v8 -t ghcr.io/nirmata/kyverno-notation-aws:v1-rc2 .
	docker push ghcr.io/nirmata/kyverno-notation-aws:v1-rc2

#############
# BUILD (KO)#
#############

LOCAL_PLATFORM      := linux/$(GOARCH)
KO_REGISTRY         := ko.local
ifndef VERSION
KO_TAGS             := $(GIT_SHA)
else ifeq ($(VERSION),main)
KO_TAGS             := $(GIT_SHA),latest
else
KO_TAGS             := $(GIT_SHA),$(subst /,-,$(VERSION))
endif

.PHONY: ko-build-kyverno-notation-aws
ko-build-kyverno-notation-aws: $(KO) ## Build kyverno-notation-aws local image (with ko)
	@echo Build kyverno-notation-aws local image with ko... >&2
	@LD_FLAGS=$(LD_FLAGS) KO_DOCKER_REPO=$(KO_REGISTRY) \
		$(KO) build ./ --preserve-import-paths --tags=$(KO_TAGS) --platform=$(LOCAL_PLATFORM)

################
# PUBLISH (KO) #
################

REGISTRY_USERNAME   ?= dummy
PLATFORMS           := linux/arm64

.PHONY: ko-login
ko-login: $(KO)
	@$(KO) login $(REGISTRY) --username $(REGISTRY_USERNAME) --password $(REGISTRY_PASSWORD)

.PHONY: ko-publish-kyverno-notation-aws
ko-publish-kyverno-notation-aws: ko-login ## Build and publish kyverno-notation-aws image (with ko)
	@LD_FLAGS=$(LD_FLAGS) KO_DOCKER_REPO=$(REPO_IMAGE) \
		$(KO) build ./ --bare --tags=$(KO_TAGS) --platform=$(PLATFORMS)
