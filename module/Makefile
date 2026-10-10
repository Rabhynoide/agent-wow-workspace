.PHONY: generate build
PROTOS := state world move quest combat auto town progress

generate:
	go build -o bin/ google.golang.org/protobuf/cmd/protoc-gen-go google.golang.org/grpc/cmd/protoc-gen-go-grpc
	@for p in $(PROTOS); do \
	  mkdir -p gen; \
	  protoc -I proto -I "$$(go list -m -f '{{.Dir}}' github.com/agent-wow/agent-wow)" \
	    --plugin=protoc-gen-go=bin/protoc-gen-go --plugin=protoc-gen-go-grpc=bin/protoc-gen-go-grpc \
	    --go_out=. --go_opt=module=wowagent --go-grpc_out=. --go-grpc_opt=module=wowagent \
	    --include_imports --descriptor_set_out=gen/$$p.pb proto/$$p.proto || exit 1; \
	  mkdir -p ../modules/$$p && cp gen/$$p.pb ../modules/$$p/module.pb; \
	done

build:
	go build ./...
