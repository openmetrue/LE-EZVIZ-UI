.PHONY: build linux vet test clean

build:
	go build -trimpath -ldflags="-s -w" -o le-ezviz-vs .
	cd web && go build -trimpath -ldflags="-s -w" -o ezvizd .

linux:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o le-ezviz-vs-linux .
	cd web && GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ezvizd-linux .

vet:
	go vet ./...
	cd web && go vet ./...

test:
	cd web && go test ./...

clean:
	rm -f le-ezviz-vs le-ezviz-vs-linux web/ezvizd web/ezvizd-linux
