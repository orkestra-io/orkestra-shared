# La cible par défaut qui génère le code
.PHONY: all generate deps clean
all: generate

deps:
	@echo "Updating Go dependencies..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.8
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go mod tidy

generate: deps
	@echo "Generating gRPC code from .proto files..."
	protoc --go_out=. --go-grpc_out=. ./proto/orkestra.proto
	@echo "gRPC code generated successfully."

clean:
	@echo "Cleaning generated gRPC files..."
	@rm -f proto/*.pb.go
	@echo "Cleanup complete."