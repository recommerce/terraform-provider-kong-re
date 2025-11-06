GOFMT_FILES?=$$(find ./ -name '*.go' | grep -v vendor)

default: build test testacc

travisbuild: deps default

test:
	TF_ACC=1 TF_LOG=DEBUG go test -v ./kong -run="TestAccKongCertificate" 2>&1 | head -100

build:
	@go build ./kong

clean:
	rm -rf pkg/

fmt:
	go fmt ./...


.PHONY: build test testacc vet goimports goimportscheck errcheck vendor-status test-compile
