.PHONY: all release

DIR=bin
OUT=seafile-cli
BUILD_DATE=`date +%Y%m%d%H%M%S`
BUILD_HASH=`git rev-parse --short HEAD`
VERSION_MAJ=0
VERSION_MIN=1

ARCHS=-osarch windows/amd64 -osarch darwin/amd64 -osarch linux/amd64 -osarch windows/386

BUILDCLI=-ldflags "-X main.buildDate=$(BUILD_DATE) -X main.buildHash=$(BUILD_HASH) -X main.verMaj=$(VERSION_MAJ) -X main.verMin=$(VERSION_MIN)"

all: $(DIR)
	go build $(BUILDCLI) -o $(DIR)/$(OUT) .

$(DIR):
	mkdir -p $@


cross:
	gox $(BUILDCLI) -output "$(DIR)/$(OUT)_{{.OS}}_{{.Arch}}" $(ARCHS)

release:
	go build -a -o $(OUT)
	goupx $(OUT)
