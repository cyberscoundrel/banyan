.PHONY: all build frontend backend clean run dev

FRONTEND_DIR := apps/chat-frontend
SERVICE_DIR := apps/chat-service
LEDGER_DIR := packages/ledger

all: build

frontend:
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) && npm install && npm run build
	rm -rf $(SERVICE_DIR)/frontend/dist
	cp -r $(FRONTEND_DIR)/dist $(SERVICE_DIR)/frontend/

backend:
	@echo "Building backend..."
	cd $(SERVICE_DIR) && go build -o chat-service .

build: frontend backend
	@echo "Build complete!"

run: build
	@echo "Running chat service..."
	cd $(SERVICE_DIR) && ./chat-service -listen :8080 -ledger-key

dev:
	@echo "Starting development servers..."
	@trap 'kill 0' INT; \
	cd $(FRONTEND_DIR) && npm run dev & \
	cd $(SERVICE_DIR) && go run . -listen :8080 -ledger-key & \
	wait

clean:
	rm -rf $(FRONTEND_DIR)/node_modules $(FRONTEND_DIR)/dist
	rm -rf $(SERVICE_DIR)/frontend/dist $(SERVICE_DIR)/chat-service
	rm -rf $(SERVICE_DIR)/data

test:
	cd $(LEDGER_DIR) && go test ./...
	cd $(SERVICE_DIR) && go test ./...
