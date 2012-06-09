# htmlfwd
## Install Go 1.0
If you use Ubuntu precise you can install Go environment with

	sudo apt-get install golang

If golang is not contained in your package management system,
follow [the instruction in golang.org](http://golang.org/doc/install).

## Install Go Websocket
Install golang websocket library.

    sudo go get code.google.com/p/go.net/websocket

Websocket library will be installed in the default directory of
go packages (/usr/lib/go/pkg).
If you don't want to (or can not) install,
you can change the install directory with GOPATH environment variable.

## Checkout and compile
	git clone https://github.com/yunabe/htmlfwd.git
	cd htmlfwd/server
	make

## Configuration
Before running htmlfwd server,
you need to create ~/.htmlfwdrc to configure port numbers.

    cat > ~/.htmlfwdrc <<EOF
    browser_port=8080
    command_port=9999
    keep_alive_interval=60
    EOF

## Start server
    ./htmlfwd

