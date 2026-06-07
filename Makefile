.PHONY: up down test sqlc lint ui build run

ui:
	cd ui && NEXT_PUBLIC_API_URL="" npm ci && NEXT_PUBLIC_API_URL="" npm run build
	rm -rf api/internal/web/dist && mkdir -p api/internal/web/dist
	cp -r ui/out/* api/internal/web/dist/

build: ui
	cd api && go build -o ../bin/pulse ./cmd/pulse

run: build
	./bin/pulse

test:
	cd api && go test ./... -count=1

sqlc:
	cd api && sqlc generate

lint:
	cd api && golangci-lint run ./...

up:
	docker compose up --build -d

down:
	docker compose down
