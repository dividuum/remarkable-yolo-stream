all: index.html stream.js main.go
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o stream

clean:
	rm -f stream

.PHONY: clean
