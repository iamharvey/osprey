run:
	mkdir -p ${HOME}/osprey/igu
	cp osprey.yml /usr/local/etc/.
	go run main.go