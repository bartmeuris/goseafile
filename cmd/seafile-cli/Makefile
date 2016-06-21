.PHONY: all release

DIR=bin
OUT=seafile-cli
ARCH=
ARCH+=-osarch windows/amd64
ARCH+=-osarch darwin/amd64
ARCH+=-osarch linux/amd64
ARCH+=-osarch windows/386
ARCH+=-osarch linux/386

all: $(DIR)
	go build -o $(DIR)/$(OUT) .

$(DIR):
	mkdir -p $@


cross:
	gox -output "$(DIR)/$(OUT)_{{.OS}}_{{.Arch}}" $(ARCH)

i386:
	gox -output "$(DIR)/$(OUT)_{{.OS}}_{{.Arch}}" -osarch linux/386

release:
	go build -a -o $(OUT)
	goupx $(OUT)
