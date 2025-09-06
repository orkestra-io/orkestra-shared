# La cible par défaut qui génère le code
.PHONY: all
all: generate

# Cible pour mettre à jour les dépendances Go et les outils de génération de code.
# Un développeur du module partagé doit exécuter cette commande s'il met à jour les versions de gRPC.
.PHONY: deps
deps:
	@echo "Updating Go gRPC tools..."
	go get -u google.golang.org/protobuf/cmd/protoc-gen-go
	go get -u google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go mod tidy

# Cible pour générer le code Go à partir des fichiers .proto.
# C'est la commande principale que le mainteneur du module doit lancer
# après chaque modification du fichier .proto.
.PHONY: generate
generate: deps
	@echo "Generating gRPC code from .proto files..."
	protoc --go_out=. --go-grpc_out=. ./proto/orkestra.proto
	@echo "gRPC code generated successfully."

# Cible pour nettoyer les fichiers générés
.PHONY: clean
clean:
	@echo "Cleaning generated gRPC files..."
	@rm -f proto/*.pb.go
	@echo "Cleanup complete."