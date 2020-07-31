.PHONY: build tomlcli

tomlcli:
	toml2cli -in-file=clifile.toml -out-file=main.go


build: tomlcli
	go build -o mirrord .
