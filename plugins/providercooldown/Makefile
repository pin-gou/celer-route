# Makefile for the providercooldown plugin image.
#
# Targets:
#   make image           — build the arm64 image and load it into the local
#                          docker daemon (single-arch). Most common usage.
#   make buildx-image    — build multi-arch with buildx and push directly
#                          to the configured registry (no local load).
#   make run             — start the image locally against $PWD/data.
#   make push            — push previously-loaded local image to registry.
#   make clean           — remove local image artifacts.
#
# Configuration:
#   IMAGE  — full image name (default myco/bifrost).
#   TAG    — image tag (default cooldown-<git short SHA>).
#   PLATFORMS — comma-separated buildx platforms (default linux/arm64).

IMAGE ?= myco/bifrost
TAG   ?= cooldown-$(shell git rev-parse --short HEAD 2>/dev/null || echo local)
PLATFORMS ?= linux/arm64

.PHONY: image buildx-image push run clean check-context help

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check-context: ## Sanity-check that the build context is the repo root
	@test -f transports/Dockerfile || { echo "ERROR: $(PWD) is not the repo root; 'cd' to the repo root first."; exit 1; }
	@test -f transports/docker-entrypoint.sh || { echo "ERROR: transports/docker-entrypoint.sh missing."; exit 1; }
	@test -f plugins/providercooldown/Dockerfile || { echo "ERROR: plugins/providercooldown/Dockerfile missing."; exit 1; }

image: check-context ## Build the arm64 image locally
	docker build \
	  -f plugins/providercooldown/Dockerfile \
	  --build-arg VERSION=$(TAG) \
	  -t $(IMAGE):$(TAG) \
	  -t $(IMAGE):latest \
	  .
	@echo "Built: $(IMAGE):$(TAG), $(IMAGE):latest"

buildx-image: check-context ## Build multi-arch and push directly (no local load)
	docker buildx build \
	  -f plugins/providercooldown/Dockerfile \
	  --platform $(PLATFORMS) \
	  --build-arg VERSION=$(TAG) \
	  -t $(IMAGE):$(TAG) \
	  --push \
	  .

push: image ## Push local image(s) to registry
	docker push $(IMAGE):$(TAG)
	docker push $(IMAGE):latest

run: image ## Run locally with a data dir on ./data (creates it if absent)
	@mkdir -p ./data
	@test -f ./data/config.json || { \
	    echo "Note: ./data/config.json missing — the container will start with defaults."; \
	    echo "  Copy a real config.json there to enable providers and plugins."; \
	}
	docker run --rm -it \
	  --platform linux/arm64 \
	  -p 8080:8080 \
	  -v $(PWD)/data:/app/data \
	  -e OPENAI_API_KEY=$${OPENAI_API_KEY:-} \
	  -e ANTHROPIC_API_KEY=$${ANTHROPIC_API_KEY:-} \
	  $(IMAGE):$(TAG)

clean: ## Remove local image artifacts
	docker rmi $(IMAGE):$(TAG) $(IMAGE):latest 2>/dev/null || true
