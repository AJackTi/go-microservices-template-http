.DEFAULT_GOAL := help

.PHONY: help up up_build down build_front build_front_linux build_broker build_auth \
	build_logger build_mail build_listener start stop

help:
	@$(MAKE) -C .. help

up:
	@$(MAKE) -C .. dev

up_build:
	@$(MAKE) -C .. dev

down:
	@$(MAKE) -C .. down

build_front build_front_linux build_broker build_auth build_logger build_mail build_listener:
	@$(MAKE) -C .. build

start:
	@$(MAKE) -C .. dev

stop:
	@$(MAKE) -C .. down
