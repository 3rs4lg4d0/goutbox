# -----------------------------------------------------------------------------
# Database parameters for local development.
# -----------------------------------------------------------------------------
DB_HOST=localhost
DB_PORT=5432
DB_NAME=goutbox
DB_USER=goutbox
DB_PASSWORD=goutbox

# -----------------------------------------------------------------------------
# Action to migrate database. Use it when you need to test your SQL scripts.
# -----------------------------------------------------------------------------
.PHONY: migrate-db
migrate-db: command := up
migrate-db: db_url := "postgresql://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"
migrate-db:
	@echo "\n-----------------------------------------------------------------------------"
	@echo "> Executing database migrations"
	@echo "-----------------------------------------------------------------------------"
	@go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate -verbose -database $(db_url) -path sql/postgres $(command)

# -----------------------------------------------------------------------------
# Actions to manage local infrastructure using Docker containers.
# -----------------------------------------------------------------------------
.PHONY: start-infra
start-infra: script-start-local-infra migrate-db

.PHONY: stop-infra
stop-infra: script-stop-local-infra

.PHONY: delete-infra
delete-infra: script-delete-local-infra

# -----------------------------------------------------------------------------
# Utility targets (don't use them directly).
# -----------------------------------------------------------------------------
.PHONY: script-start-local-infra
script-start-local-infra:
	@echo "\n-----------------------------------------------------------------------------"
	@echo "> Starting docker containers"
	@echo "-----------------------------------------------------------------------------"
	@export POSTGRES_DB=$(DB_NAME) && \
	export POSTGRES_USER=$(DB_USER) && \
	export POSTGRES_PASSWORD=$(DB_PASSWORD) && \
	scripts/infra/start-local-infra.sh

.PHONY: script-start-local-infra
script-stop-local-infra:
	@echo "\n-----------------------------------------------------------------------------"
	@echo "> Stopping docker containers"
	@echo "-----------------------------------------------------------------------------"
	@export POSTGRES_DB=$(DB_NAME) && \
	export POSTGRES_USER=$(DB_USER) && \
	export POSTGRES_PASSWORD=$(DB_PASSWORD) && \
	scripts/infra/stop-local-infra.sh

.PHONY: script-delete-local-infra
script-delete-local-infra:
	@echo "\n-----------------------------------------------------------------------------"
	@echo "> Deleting docker containers"
	@echo "-----------------------------------------------------------------------------"
	@export POSTGRES_DB=$(DB_NAME) && \
	export POSTGRES_USER=$(DB_USER) && \
	export POSTGRES_PASSWORD=$(DB_PASSWORD) && \
	scripts/infra/delete-local-infra.sh