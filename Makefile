SHELL := bash

default:

test:
	go test -v -race -coverprofile=coverprofile.out ./...

update-deps:
	hack/update-deps.sh

coverprofile:
	hack/coverprofile.sh

lint:
	golangci-lint run -v

fmt:
	gofmt -s -w ./promremote

validate:
	hack/validate.sh

clean:
	rm -rf coverprofiles coverprofile.out

.PHONY: \
	default \
	test \
	update-deps \
	coverprofile \
	lint \
	fmt \
	validate \
	clean \
	$(NULL)
