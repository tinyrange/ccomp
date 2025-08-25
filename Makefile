GOCACHE := $(PWD)/.cache/go-build
GOMODCACHE := $(PWD)/.cache/gomod

.PHONY: build run clean e2e

build:
	@mkdir -p $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go build -o ccomp ./cmd/ccomp

run: build
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) ./ccomp -o out.s examples/phase1/ret_expr.c
	@echo "wrote out.s"

e2e: run
	gcc -nostdlib out.s runtime/start_linux_amd64.s -o a.out
	./a.out; code=$$?; echo "exit=$$code"; test $$code -eq 14

clean:
	rm -f ccomp out.s
	rm -rf .cache
