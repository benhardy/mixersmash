.PHONY: all linux test clean

all: go2d_darwin_arm64

go2d_darwin_arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o go2d_darwin_arm64 .

linux: go2d_linux_amd64

go2d_linux_amd64:
	GOOS=linux GOARCH=amd64 go build -o go2d_linux_amd64 .

test:
	go test ./...

clean:
	rm -f go2d_darwin_arm64 go2d_linux_amd64
