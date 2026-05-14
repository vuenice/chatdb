.PHONY: frontend backend build clean dev-backend dev-frontend

# Build the Vue SPA into backend/web/dist for embedding.
frontend:
	cd frontend && npm install && npm run build
	rm -rf backend/web/dist
	mkdir -p backend/web/dist
	cp -R frontend/dist/. backend/web/dist/

# Compile a single static Linux binary at ./chatdb that serves the API + SPA.
backend:
	cd backend && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../chatdb ./cmd/chatdb

# Full release build: SPA + binary.
build: frontend backend
	@echo "Built ./chatdb (first run creates config under OS user config dir; see README)"

clean:
	rm -rf chatdb backend/web/dist frontend/dist
	mkdir -p backend/web/dist
	echo "placeholder so go:embed has at least one file" > backend/web/dist/.gitkeep

dev-backend:
	cd backend && go run ./cmd/chatdb

dev-frontend:
	cd frontend && npm run dev
