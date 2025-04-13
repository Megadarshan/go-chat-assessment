IMAGE_NAME=local/go-chat-app:0.1
DOCKERFILE=go-chatApp.dockerfile

BINARY_NAME=goChatApp

build_binary:
	@echo "Building binary..."
	env GOOS=linux CGO_ENABLED=0 go build -o ${BINARY_NAME} .
	@echo "Done"

# Build Docker image
docker_build: build_binary
	DOCKER_BUILDKIT=0 docker build -t $(IMAGE_NAME) -f $(DOCKERFILE) .

## up: start docker compose
up:
	@echo "Starting Docker images..."
	docker-compose up -d
	@echo "Docker images started!"

## down: stop docker compose
down:
	@echo "Stopping docker compose..."
	docker-compose down
	@echo "Done!"