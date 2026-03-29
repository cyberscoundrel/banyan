.PHONY: all build frontend backend clean run dev test

FRONTEND_DIR := ../chat-frontend
SERVICE_DIR := .

all: build

frontend:
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) && npm install && npm run build
	rm -rf frontend/dist
	cp -r $(FRONTEND_DIR)/dist frontend/

backend:
	@echo "Building backend..."
	npm install
	npm run build

build: frontend backend
	@echo "Build complete!"

run: build
	@echo "Running chat service..."
	node dist/index.js --listen :8080 --ledger-key

dev:
	@echo "Starting development servers..."
	@trap 'kill 0' INT; \
	cd $(FRONTEND_DIR) && npm run dev & \
	npm run dev & \
	wait

clean:
	rm -rf $(FRONTEND_DIR)/node_modules $(FRONTEND_DIR)/dist
	rm -rf frontend/dist dist node_modules data

test:
	npm test
