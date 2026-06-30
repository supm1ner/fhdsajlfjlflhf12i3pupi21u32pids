# Convenience targets for the Sunrise full stack.
.PHONY: up down logs ps build register smoke clean nuke

up:        ## Build images and start the whole stack
	docker compose up -d --build

down:      ## Stop the stack (keep data)
	docker compose down

logs:      ## Tail logs of all services
	docker compose logs -f

ps:        ## Show service status
	docker compose ps

build:     ## Build images without starting
	docker compose build

register:  ## Register the messenger OAuth client in Hydra (run after `up`)
	./register-rp.sh

smoke:     ## Quick health probes
	@echo "Hydra:    $$(curl -s -o /dev/null -w '%{http_code}' http://localhost:4445/health/ready)"
	@echo "cotton-id:$$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/healthz)"
	@echo "sunrise:  $$(curl -s -o /dev/null -w '%{http_code}' http://localhost:6060/)"
	@echo "web:      $$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8088/)"
	@echo "livekit:  $$(curl -s -o /dev/null -w '%{http_code}' http://localhost:7880/)"

clean:     ## Stop and remove containers + networks (keep volumes)
	docker compose down --remove-orphans

nuke:      ## Stop and DELETE all data volumes (full reset)
	docker compose down -v --remove-orphans
