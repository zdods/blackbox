.PHONY: build-bastion build-daemon up dev test clean logs down

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMPOSE_DEV = docker compose -f docker-compose.yml -f docker-compose.dev.yml

build-bastion:
	$(COMPOSE_DEV) build bastion

build-daemon:
	go build -ldflags "-X blackhaul/pkg/version.Version=$(VERSION)" -o blackhaul-daemon ./daemon

up:
	docker compose up

dev:
	DEV_MODE=1 $(COMPOSE_DEV) up --build --watch

test:
	go test ./bastion/ ./daemon/ ./pkg/...

clean:
	rm -f blackhaul-daemon
	docker compose down -v --remove-orphans

logs:
	docker compose logs -f

down:
	docker compose down
