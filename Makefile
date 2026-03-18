.PHONY: build build-agent build-hub dev clean deps

AGENT_BIN = bin/agent
HUB_BIN   = bin/hub

deps:
	go mod tidy

build: build-agent build-hub

build-agent:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(AGENT_BIN) ./cmd/agent

build-hub:
	go build -o $(HUB_BIN) ./cmd/hub

# Cross-compile agent for deployment to Linodes
build-agent-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(AGENT_BIN)-linux ./cmd/agent

# Run hub locally for development
dev:
	LISTEN_ADDR=:8080 go run ./cmd/hub

# Run a test agent locally
dev-agent:
	REGION=local NATS_URL=nats://127.0.0.1:4222 LISTEN_ADDR=:9090 go run ./cmd/agent

clean:
	rm -rf bin/

# Terraform targets
deploy-hub:
	cd infra/terraform && terraform apply -target=module.hub

deploy-agents:
	cd infra/terraform && terraform apply -target=module.agent

deploy:
	cd infra/terraform && terraform apply

destroy:
	cd infra/terraform && terraform destroy
	@echo "REMINDER: Check for orphaned NodeBalancers and Volumes"
	@echo "  linode-cli nodebalancers list"
	@echo "  linode-cli volumes list"

up: deploy
down: destroy
