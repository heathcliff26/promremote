SHELL := bash

default:

test:
	go test -v -race ./...

update-deps:
	hack/update-deps.sh

coverprofile:
	hack/coverprofile.sh

lint:
	golangci-lint run -v

.PHONY: \
	default \
	test \
	update-deps \
	coverprofile \
	lint \
	$(NULL)
