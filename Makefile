GOCACHE := $(PWD)/.cache/go-build
GOMODCACHE := $(PWD)/.cache/gomod

.PHONY: build run clean e2e test

build:
	@mkdir -p $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go build -o ccomp ./cmd/ccomp

clean:
	rm -f ccomp out.s a.out .test.s .test.bin .t .t.s .w .w.s
	rm -rf .cache .test-tmp

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) bash tools/run_tests.sh
