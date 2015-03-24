.PHONY: all release

DIR=bin
OUT=seafile-cli

all: $(DIR)
	go build -o $(DIR)/$(OUT) .

$(DIR):
	mkdir -p $@


cross:
	gox -output "$(DIR)/$(OUT)_{{.OS}}_{{.Arch}}" -osarch windows/amd64 -osarch darwin/amd64 -osarch linux/amd64 -osarch windows/386

release:
	go build -a -o $(OUT)
	goupx $(OUT)
