.PHONY: ui test vet build demo kubernetes-build clean deploy-remote

ui:
	./scripts/sync-ui.sh

test: ui
	go test ./cmd/... ./internal/...

vet: ui
	go vet ./cmd/... ./internal/...

build: ui test vet
	mkdir -p bin
	go build -o bin/zyvor-axiom ./cmd/zyvor-axiom

demo: build
	AXIOM_MODE=demo AXIOM_USERNAME=demo AXIOM_PASSWORD=demo AXIOM_ADDR=:5090 ./bin/zyvor-axiom

kubernetes-build: ui
	mkdir -p bin
	go build -modfile=go.production.mod -tags kubernetes -o bin/zyvor-axiom-kubernetes ./cmd/zyvor-axiom

deploy-remote:
	./scripts/deploy-remote.sh $(H) $(U)

clean:
	rm -rf bin
