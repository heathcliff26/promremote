SHELL := bash

default: test

# Run unit-tests
test:
	go test -v -race -coverprofile=coverprofile.out ./...

# Update project dependencies
update-deps:
	hack/update-deps.sh

# Generate coverage profile
coverprofile:
	hack/coverprofile.sh

# Run linter
lint:
	golangci-lint run -v

# Format the codebase
fmt:
	gofmt -s -w ./promremote

# Validate generated files are up-to-date
validate:
	hack/validate.sh

# Scan code for vulnerabilities using gosec
gosec:
	gosec ./...

# Remove generated files and artifacts
clean:
	rm -rf coverprofiles coverprofile.out

# Show this help message
help:
	@echo "Available targets:"
	@echo ""
	@awk '/^#/{c=substr($$0,3);next}c&&/^[[:alpha:]][[:alnum:]_-]+:/{print substr($$1,1,index($$1,":")),c}1{c=0}' $(MAKEFILE_LIST) | column -s: -t
	@echo ""
	@echo "Run 'make <target>' to execute a specific target."

.PHONY: \
	default \
	test \
	update-deps \
	coverprofile \
	lint \
	fmt \
	validate \
	gosec \
	clean \
	help \
	$(NULL)
