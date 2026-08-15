.PHONY: DOWN, BUILD, RUN, NO DB CLEANUP
build-and-run:
	docker compose down --remove-orphans
	docker compose up --build -d
