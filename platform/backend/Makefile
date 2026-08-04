.PHONY: build run test clean

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o alcatraz ./cmd/alcatraz

run: build
	./alcatraz

test:
	go test ./... -count=1

clean:
	rm -f alcatraz
