.PHONY: build-bastion build-agent up dev

build-bastion:
	docker compose build bastion

build-agent:
	go build -o blackbox-agent ./agent

up:
	docker compose up --build

dev:
	docker compose up --build --watch
