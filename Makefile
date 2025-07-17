############
# DEFAULTS #
############

GIT_SHA              := $(shell git rev-parse HEAD)
REGISTRY             ?= ghcr.io
REPO                 ?= nirmata
IMAGENAME                ?= kyverno-notation-aws
GOOS                 ?= $(shell go env GOOS)
GOARCH               ?= $(shell go env GOARCH)
CGO_ENABLED          ?= 0
REPO_IMAGE           := $(REGISTRY)/$(REPO)/$(IMAGENAME)
KIND_IMAGE           ?= kindest/node:v1.33.1
KIND_NAME            ?= kind
KIND_CONFIG	         ?= default
BUILD_WITH           ?= docker


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
KIND                               ?= $(TOOLS_DIR)/kind
KIND_VERSION                       ?= v0.29.0
ifeq ($(GOOS), darwin)
SED                                := gsed
else
SED                                := sed
endif
KUBE_VERSION		 ?= v1.25.0

$(GO_ACC):
	@echo Install go-acc... >&2
	@GOBIN=$(TOOLS_DIR) go install github.com/ory/go-acc@$(GO_ACC_VERSION)

$(KO):
	@echo Install ko... >&2
	@GOBIN=$(TOOLS_DIR) go install github.com/google/ko@$(KO_VERSION)

$(HELM):
	@echo Install helm... >&2
	@GOBIN=$(TOOLS_DIR) go install helm.sh/helm/v3/cmd/helm@$(HELM_VERSION)

$(KIND):
	@echo Install kind... >&2
	@GOBIN=$(TOOLS_DIR) go install sigs.k8s.io/kind@$(KIND_VERSION)

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

CMD_DIR       := cmd
KYVERNO_DIR   := $(CMD_DIR)/kyverno
IMAGE_TAG_SHA := $(GIT_SHA)
IMAGE_TAG     ?= latest
PACKAGE       ?= github.com/nirmata/kyverno-notation-aws
ifdef VERSION
LD_FLAGS      := "-s -w -X $(PACKAGE)/pkg/version.BuildVersion=$(VERSION)"
else
LD_FLAGS      := "-s -w"
endif

build:
	go build -o kyverno-notation-aws

#################
# BUILD (DOCKER)#
#################

docker-build:
	@echo Build kyverno-notation-aws image with docker... >&2
	docker buildx build -t $(REPO_IMAGE):$(IMAGE_TAG) . --load

docker-publish:
	@echo Build kyverno-notation-aws image with docker... >&2
	docker buildx create --name multiarch --driver docker-container --use
	docker buildx build --platform linux/amd64,linux/arm64 -t $(REPO_IMAGE):$(IMAGE_TAG) --push .
	docker buildx rm multiarch

t:
	@echo $(IMAGE_TAG)

#################
# BUILD (IMAGE) #
#################
image-build: $(BUILD_WITH)-build

########
# HELM #
########

.PHONY: codegen-helm-docs
codegen-helm-docs: ## Generate helm docs
	@echo Generate helm docs... >&2
	@docker run -v ${PWD}/charts:/work -w /work jnorwood/helm-docs:v1.11.0 -s file

.PHONY: install-kyverno-notation-aws
install-kyverno-notation-aws: $(HELM) ## Install kyverno notation AWS helm chart
	@echo Install kyverno-notation-aws chart... >&2
	@$(HELM) upgrade --install kyverno-notation-aws --namespace kyverno-notation-aws --create-namespace --wait ./charts/kyverno-notation-aws

.PHONY: helm-setup-openreports
helm-setup-openreports: $(HELM) ## Add openreports helm repo and build dependencies
	@$(HELM) repo add openreports https://openreports.github.io/reports-api
	@$(HELM) dependency build ./charts/kyverno-notation-aws

#############
# HELM TEST #
#############

.PHONY: helm-test
helm-test: $(HELM) ## Run Helm tests
	@echo Running helm tests... >&2
	@$(HELM) dependency build ./charts/kyverno-notation-aws
	@$(HELM) test --namespace kyverno-notation-aws kyverno-notation-aws


########
# KIND #
########

.PHONY: kind-create-cluster
kind-create-cluster: $(KIND) ## Create kind cluster
	@echo Create kind cluster... >&2
	@$(KIND) create cluster --name $(KIND_NAME) --image $(KIND_IMAGE) --config ./scripts/config/kind/$(KIND_CONFIG).yaml

.PHONY: kind-delete-cluster
kind-delete-cluster: $(KIND) ## Delete kind cluster
	@echo Delete kind cluster... >&2
	@$(KIND) delete cluster --name $(KIND_NAME)

.PHONY: kind-load-image
kind-load-image: $(KIND) image-build ## Build kyverno-notation-aws image and load it inside kind
	@echo Load kyverno-notation-aws image... >&2
	@$(KIND) load docker-image --name $(KIND_NAME) $(REPO_IMAGE):$(GIT_SHA)

.PHONY: kind-deploy-image
kind-deploy-image: $(HELM) kind-load-image ## Build image, load it inside kind cluster and deploy the helm chart
	@$(MAKE) kind-install-image

.PHONY: kind-install-image
kind-install-image: $(HELM) helm-setup-openreports ## Install helm-chart
	@echo Installing kyverno-notation-aws helm chart
	@$(HELM) upgrade --install kyverno-notation-aws --namespace kyverno-notation-aws --create-namespace --wait ./charts/kyverno-notation-aws


###########
# CODEGEN #
###########
.PHONY: codegen-manifest-release
codegen-manifest-release: ## Create release manifest
codegen-manifest-release: $(HELM)
codegen-manifest-release:
	@echo Generate release manifest... >&2
	@mkdir -p ./.manifest
	@$(HELM) template kyverno-notation-aws --kube-version $(KUBE_VERSION) --namespace kyverno-notation-aws --skip-tests ./charts/kyverno-notation-aws \
		--set image.tag=$(VERSION) \
 		| $(SED) -e '/^#.*/d' \
		> ./.manifest/release.yaml
