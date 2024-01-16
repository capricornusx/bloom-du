.PHONY: install-go-test-coverage
install-go-test-coverage:
	go install github.com/vladopajic/go-test-coverage/v2@latest

.PHONY: check-coverage
check-coverage: install-go-test-coverage
	CGO_ENABLED=1 GOEXPERIMENT=loopvar go test -race -coverprofile=./cover.out -covermode=atomic ./...

PROJECT_PREFIX_1=capricornusx/bloom-du/
PROJECT_PREFIX_2=bloom-du/

all: test build

vet:
	go vet ./...

test: format tidy vet
	CGO_ENABLED=1 GOEXPERIMENT=loopvar go test -race ./...

check: format #govulncheck gosec

gocritic:
	gocritic check --disable=commentFormatting ./...

govulncheck:
	govulncheck ./...

gosec:
	gosec -exclude=G505 ./...

format: gci
	gofmt -w .

gci:
	gci write -s standard -s default -s "prefix(${PROJECT_PREFIX_1})" -s "prefix(${PROJECT_PREFIX_2})" -s blank -s dot .

tidy:
	go mod tidy

build: format tidy inc
	CGO_ENABLED=0 GOOS="linux" GOARCH="amd64" go install --tags fts5 ./cmd/magneticod
