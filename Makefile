SHELL := /bin/sh

-include .env

export DB_HOST ?= localhost
export DB_PORT ?= 5432
export DB_USER ?= postgres
export DB_PASSWORD ?= password
export DB_NAME ?= auth_api
export JWT_SECRET ?= replace-with-a-strong-secret-key
export REDIS_HOST ?= localhost
export REDIS_PORT ?= 6379
export REDIS_PASSWORD ?=
export REDIS_DB ?= 0

GO ?= go
MIGRATE ?= migrate
DOCKER_COMPOSE ?= docker compose
DB_URL := postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable

.PHONY: help up down restart logs ps run test test-race migrate-up migrate-down migrate-force

help:
	@echo "available targets:"
	@echo "  make up            - start docker services in background"
	@echo "  make down          - stop docker services"
	@echo "  make restart       - restart docker services"
	@echo "  make logs          - tail docker compose logs"
	@echo "  make ps            - show docker compose services"
	@echo "  make run           - run the api locally with current env"
	@echo "  make test          - run all go tests"
	@echo "  make test-race     - run go tests with race detector"
	@echo "  make migrate-up    - apply all migrations"
	@echo "  make migrate-down  - rollback one migration"
	@echo "  make migrate-force version=<n> - force migration version"

up:
	$(DOCKER_COMPOSE) up --build -d

down:
	$(DOCKER_COMPOSE) down

restart: down up

logs:
	$(DOCKER_COMPOSE) logs -f

ps:
	$(DOCKER_COMPOSE) ps

run:
	$(GO) run main.go

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

migrate-up:
	$(MIGRATE) -path migrations -database "$(DB_URL)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(DB_URL)" down 1

migrate-force:
	@if [ -z "$(version)" ]; then echo "usage: make migrate-force version=<n>"; exit 1; fi
	$(MIGRATE) -path migrations -database "$(DB_URL)" force $(version)
