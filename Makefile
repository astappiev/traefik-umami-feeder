.PHONY: default lint test yaegi_test vendor clean generate tidy

default: lint test

lint:
	golangci-lint run

test:
	go test -race -cover ./...

yaegi_test:
	yaegi test -v .

vendor:
	go mod vendor

clean:
	rm -rf ./vendor

generate:
	go generate ./...

tidy:
	go mod tidy
