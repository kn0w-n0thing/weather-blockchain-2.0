# Weather Blockchain 2.0 Makefile

# Run the main blockchain application
.PHONY: run
run:
	go run main.go $(if $(ARGS),$(ARGS),)

# Run all tests
.PHONY: test
test:
	go test ./...