# local development: made-up secrets, override with e.g. make run PARENT_PIN=4321
FAMILY_PASSWORD ?= family
PARENT_PIN ?= 1234

.PHONY: run
run:
	FAMILY_PASSWORD=$(FAMILY_PASSWORD) PARENT_PIN=$(PARENT_PIN) go run .
