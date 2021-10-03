#!/bin/sh

for os in windows linux darwin ; do
	extension=""
	if [ "$os" = "windows" ]; then extension=".exe"; fi
	GOOS=$os go build -o ./bin/$os/clashcli"$extension"
done
