.PHONY: up down test cover cover-html sqlc lint ui build run

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

# Coverage gate (excludes internal/generated). Override module/threshold:
#   make cover MODULE=agent THRESHOLD=90
MODULE ?= api
THRESHOLD ?= 90
cover:
	bash scripts/coverage.sh $(MODULE) $(THRESHOLD)

cover-html:
	cd $(MODULE) && go test ./... -covermode=atomic -coverprofile=coverage.out >/dev/null && \
		grep -v internal/generated/ coverage.out > coverage.nogen.out && \
		go tool cover -html=coverage.nogen.out -o coverage.html && \
		echo "wrote $(MODULE)/coverage.html"

sqlc:
	cd api/internal/db && sqlc generate

lint:
	cd api && golangci-lint run ./...

up:
	docker compose up --build -d

down:
	docker compose down
