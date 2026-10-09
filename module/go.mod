module wowagent

go 1.27.1

require (
	github.com/agent-wow/agent-wow v0.1.0
	github.com/go-sql-driver/mysql v1.10.1
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.10
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
)

tool (
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
