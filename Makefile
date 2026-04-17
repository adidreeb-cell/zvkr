APP=zvkr
BUILD_DIR=build

FRONTEND_DIR=frontend
FRONTEND_DIST=$(FRONTEND_DIR)/dist
BACKEND_DIST=internal/routes/dist

GOFLAGS=-trimpath -ldflags="-s -w -buildid="

.PHONY: all clean frontend copy-frontend backend build

all: clean frontend copy-frontend backend

clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(BACKEND_DIST)

frontend:
	cd $(FRONTEND_DIR) && bun install
	cd $(FRONTEND_DIR) && bun run tsc --noEmit
	cd $(FRONTEND_DIR) && bun run vite build

copy-frontend:
	mkdir -p $(BACKEND_DIST)
	cp -r $(FRONTEND_DIST)/* $(BACKEND_DIST)/

backend:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOEXPERIMENT=greenteagc \
	go build $(GOFLAGS) -o $(BUILD_DIR)/$(APP) cmd/gate/main.go

build: all
