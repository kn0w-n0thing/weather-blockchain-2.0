# Weather Blockchain 2.0 Makefile

# Run the main blockchain application
.PHONY: run
run:
	go run main.go $(if $(ARGS),$(ARGS),)

# Run weather service test
.PHONY: test_weather
test_weather:
	go run test_weather.go

# Run all tests
.PHONY: test
test:
	go test ./...