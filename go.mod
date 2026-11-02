module github.com/sinni19/go-svclib

go 1.25.0

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20260709200747-435963d16310.1
	buf.build/go/protovalidate v1.3.0
	connectrpc.com/connect v1.21.0
	connectrpc.com/vanguard v0.4.0
	github.com/caarlos0/env/v11 v11.4.1
	go.uber.org/goleak v1.3.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260223185530-2f722ef697dc
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260223185530-2f722ef697dc
	google.golang.org/protobuf v1.36.11
)

require (
	cel.dev/expr v0.25.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/google/cel-go v0.30.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20250813145105-42675adae3e6 // indirect
	golang.org/x/text v0.28.0 // indirect
)

tool (
	connectrpc.com/connect/cmd/protoc-gen-connect-go
	google.golang.org/protobuf/cmd/protoc-gen-go
)
