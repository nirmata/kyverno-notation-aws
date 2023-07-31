#########
# TOOLS #
#########

TOOLS_DIR                          := $(PWD)/.tools
GO_ACC                             := $(TOOLS_DIR)/go-acc
GO_ACC_VERSION                     := latest
TOOLS                              := $(GO_ACC)

$(GO_ACC):
	@echo Install go-acc... >&2
	@GOBIN=$(TOOLS_DIR) go install github.com/ory/go-acc@$(GO_ACC_VERSION)

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

#########
# BUILD #
#########

build:
	go build -o kyverno-notation-aws

docker:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o kyverno-notation-aws .
	docker buildx build --platform linux/arm64/v8 -t ghcr.io/nirmata/kyverno-notation-aws:v1-rc2 .
	docker push ghcr.io/nirmata/kyverno-notation-aws:v1-rc2
