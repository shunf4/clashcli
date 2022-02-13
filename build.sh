#!/bin/sh

for os in windows linux darwin ; do
	extension=""
	if [ "$os" = "windows" ]; then extension=".exe"; fi
	GOOS=$os go build -o ./bin/$os/clashcli"$extension"
done

GOOS=linux GOARCH=arm64 go build -o ./bin/linux-arm64/clashcli
