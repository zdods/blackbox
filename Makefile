.PHONY: build-bastion build-daemon up dev test clean logs down

build-bastion:
	docker compose build bastion

build-daemon:
	go build -o blackbox-daemon ./daemon

up:
	docker compose up --build

dev:
	DEV_MODE=1 docker compose up --build --watch

test:
	go test ./bastion/ ./daemon/ ./pkg/...

clean:
	rm -f blackbox-daemon
	docker compose down -v --remove-orphans

logs:
	docker compose logs -f

down:
	docker compose down
