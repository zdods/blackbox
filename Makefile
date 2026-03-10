.PHONY: build-bastion build-daemon up dev test

build-bastion:
	docker compose build bastion

build-daemon:
	go build -o blackbox-daemon ./daemon

up:
	docker compose up --build

dev:
	docker compose up --build --watch

test:
	go test ./bastion/ ./daemon/ ./pkg/...
